package app

import (
	"agent-p/handle"

	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

const (
	composeVersion = "3.9"
)

type Job struct {
	Name            string
	ExpectedRunTime int
}

type DockerCompose struct {
	Version  string             `yaml:"version"`
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image       string   `yaml:"image"`
	Ports       []string `yaml:"ports,omitempty"`
	DependsOn   []string `yaml:"depends_on,omitempty"`
	Environment []string `yaml:"environment"`
}

const (
	JobsDir           = "jobs"
	DataDir           = "data"
	DefaultSampleRate = 1
)

type Batch []Job

func (b Batch) Run(clean bool) {
	log.Info().Msg("Running jobs...")
	for _, job := range b {
		job.run()

		if clean {
			job.Clean()
		}
	}
}

func (c *RunConfig) Clean() {
	for _, run := range c.Runs {
		doClean(run.Name)
	}
}

func (j *Job) Clean() {
	doClean(j.Name)
}

func doClean(name string) {
	cmd := exec.Command("docker", "compose", "-f", fmt.Sprintf("%s/%s/docker-compose.yaml", JobsDir, name), "down")
	log.Debug().Msg(cmd.String())
	err := cmd.Run()
	if err != nil {
		log.Debug().Msgf("docker compose failed to clean resources for job")
		handle.InternalError(err)
	}
	log.Info().Msgf("Job %s cleaned", name)
}

func (j *Job) run() {
	cmd := exec.Command("docker", "compose", "-f", fmt.Sprintf("%s/%s/docker-compose.yaml", JobsDir, j.Name), "up", "-d")
	log.Debug().Msgf("running job %s: %s", j.Name, cmd.String())
	err := cmd.Run()
	if err != nil {
		log.Debug().Msgf("docker compose failed to run job")
		handle.InternalError(err)
	}

	appID, driverID := j.getContainerIDs()
	log.Debug().Msgf("app: %s\ndriver: %s", appID, driverID)
	j.Monitor(DefaultSampleRate, appID, driverID)
}

func (j *Job) getContainerIDs() (appID, driverID string) {
	cmd := exec.Command("docker", "compose", "-f", fmt.Sprintf("%s/%s/docker-compose.yaml", JobsDir, j.Name), "ps", "--format", "json")
	log.Debug().Msgf("getting container ID's for job %s: %s", j.Name, cmd.String())
	out, err := cmd.Output()
	if err != nil {
		handle.InternalError(err)
	}
	containers := []types.Container{}
	json.Unmarshal(out, &containers)

	if len(containers) != 2 {
		handle.InternalError(fmt.Errorf("expecting only an app and driver container for job \"%s\", got: %v", j.Name, containers))
	}

	return containers[0].ID, containers[1].ID
}

func (j *Job) Monitor(collectionIntervalSecond int, appID, driverID string) {
	log.Debug().Msgf("monitoring and gathering data for job \"%s\"...", j.Name)
	cli, err := client.NewClientWithOpts()
	if err != nil {
		handle.InternalError(err)
	}

	fileName := fmt.Sprintf("./%s/%s/data.csv", JobsDir, j.Name)
	dataFile, err := os.Create(fileName)
	if err != nil {
		handle.InternalError(err)
	}
	defer dataFile.Close()

	data := bufio.NewWriter(dataFile)
	data.WriteString("Time, CPU utilization %, Memory Usage Mb, Disk Write Kb, Outbound Network Traffic Kb per " + fmt.Sprint(collectionIntervalSecond) + "s\n")

	var previousTx, previousCPU, previousSystem float64

	driverNotRunning := false
	ticker := time.NewTicker(time.Duration(collectionIntervalSecond) * time.Second)
	go watchContainer(cli, driverID, j.ExpectedRunTime+20, &driverNotRunning)
	for range ticker.C {
		stats := getStats(cli, appID)
		cpuPercent := calculateCPUPercentUnix(previousCPU, previousSystem, &stats)
		previousCPU = float64(stats.CPUStats.CPUUsage.TotalUsage)
		previousSystem = float64(stats.CPUStats.SystemUsage)
		_, tx := calculateNetwork(stats.Networks)
		txDiff := tx - previousTx
		previousTx = tx

		data.WriteString(time.Now().String())
		data.WriteByte(',')
		data.WriteString(fmt.Sprintf("%.3f,", cpuPercent))
		data.WriteString(fmt.Sprintf("%.3f,", (float64(stats.MemoryStats.Usage)/1024)/1024))
		data.WriteString(fmt.Sprintf("%.3f,", float64(stats.StorageStats.WriteSizeBytes)/1024))
		data.WriteString(fmt.Sprintf("%.3f", txDiff/1024))
		data.WriteString("\n")
		if driverNotRunning {
			break
		}
	}

	log.Debug().Msgf("writing captued data to file: %s...", dataFile)
	err = data.Flush()
	if err != nil {
		handle.InternalError(err)
	}

	log.Debug().Msgf("done monitoring job %s", j.Name)

	timeout := time.Millisecond * 300
	cli.ContainerStop(context.Background(), appID, &timeout)
}

func calculateCPUPercentUnix(previousCPU, previousSystem float64, v *types.StatsJSON) float64 {

	cpuPercent := 0.0
	// calculate the change for the cpu usage of the container in between readings
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - previousCPU
	// calculate the change for the entire system between readings
	systemDelta := float64(v.CPUStats.SystemUsage) - previousSystem

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(v.CPUStats.OnlineCPUs) * 100.0
		//		log.Debug().Msgf("cpu delta: %.2f\nsystem delta: %.2f\ncores: %d", cpuDelta, systemDelta, v.CPUStats.OnlineCPUs)
		//		log.Debug().Msgf("cpu usage: %.2f", cpuPercent)
	}
	return cpuPercent
}

func calculateNetwork(network map[string]types.NetworkStats) (float64, float64) {
	var rx, tx float64

	for _, v := range network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return rx, tx
}

func watchContainer(client *client.Client, containerID string, timeout int, finished *bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(timeout))
	defer cancel()

	log.Debug().Msgf("waiting for container %s to exit...", containerID)
	bC, _ := client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	<-bC
	*finished = true
}

func getStats(cli *client.Client, containerName string) types.StatsJSON {
	statsReader, err := cli.ContainerStatsOneShot(context.TODO(), containerName)
	if err != nil {
		handle.InternalError(err)
	}

	defer statsReader.Body.Close()

	stats := types.StatsJSON{}
	buf, err := ioutil.ReadAll(statsReader.Body)
	if err != nil {
		handle.InternalError(err)
	}
	err = json.Unmarshal(buf, &stats)
	if err != nil {
		handle.InternalError(err)
	}

	return stats
}

// WriteFile writes a docker compose file to your local disk
func (compose *DockerCompose) WriteFile(name string) error {
	log.Debug().Msgf("Writing Docker Compose File for Job \"%s\"...", name)
	yaml, err := yaml.Marshal(compose)
	if err != nil {
		handle.InternalError(err)
	}

	// Make jobs Dir
	err = mkdirIfNotExists("./", JobsDir)
	if err != nil {
		handle.InternalError(err)
	}

	// Make directory for job
	err = mkdirIfNotExists(fmt.Sprintf("./%s/", JobsDir), name)
	if err != nil {
		handle.InternalError(err)
	}

	// Overwrite files that already exist
	composeFile := fmt.Sprintf("./%s/%s/docker-compose.yaml", JobsDir, name)
	f, err := os.Create(composeFile)
	if err != nil {
		handle.InternalError(err)
	}

	_, err = f.Write(yaml)
	if err != nil {
		handle.InternalError(err)
	}

	f.Close()

	log.Debug().Msgf("compose file for job \"%s\" written: %s", name, composeFile)
	return nil
}

func mkdirIfNotExists(path, name string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		handle.InternalError(err)
	}

	for _, file := range files {
		if file.Name() == name && file.IsDir() {
			log.Debug().Msgf("file \"%s\" exists, and will not be created...", path+name)
			return nil
		}
	}

	log.Debug().Msgf("creating file \"%s\"...", path+name)
	return os.Mkdir(path+name, os.ModePerm)
}
