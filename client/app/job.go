package app

import (
	"agent-p/handle"
	"io"
	"math/rand"

	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

const (
	composeVersion = "3.9"
)

type Job struct {
	SummaryStatisticsData  bool
	Name                   string
	Directory              JobDirectory
	ExpectedRunTime        time.Duration
	LoadDuration           time.Duration
	LoadDelay              time.Duration
	DataCollectionInterval time.Duration
}

const (
	JobsDir = "jobs"
	DataDir = "data"
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
		jobDir := ToLocalJobDirectory(run.Name)
		doClean(jobDir)
	}
}

func (j *Job) Clean() {
	doClean(j.Directory)
}

func doClean(j JobDirectory) {
	cmd := exec.Command("docker", "compose", "-f", j.GetCompose(), "down")
	log.Debug().Msg(cmd.String())
	err := cmd.Run()
	if err != nil {
		handle.DockerComposeError(cmd.String())
	}
	log.Info().Msgf("cleaned up docker containers")
}

func (j *Job) run() {
	log.Debug().Msgf("running job %+v", j)
	cmd := exec.Command("docker", "compose", "-f", j.Directory.GetCompose(), "up", "-d")
	log.Debug().Msgf("running job %s: %s", j.Name, cmd.String())
	err := cmd.Run()
	if err != nil {
		log.Debug().Msg(err.Error())
		handle.DockerComposeError(cmd.String())
	}

	appID, driverID := j.getContainerIDs()
	log.Debug().Msgf("app: %s\ndriver: %s", appID, driverID)
	j.Monitor(appID, driverID)
}

func (j *Job) getContainerIDs() (appID, driverID string) {
	cmd := exec.Command("docker", "compose", "-f", j.Directory.GetCompose(), "ps", "--format", "json")
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

func (j *Job) Monitor(appID, driverID string) {
	log.Debug().Msgf("monitoring and gathering data for job \"%s\"...", j.Name)
	cli, err := client.NewClientWithOpts()
	if err != nil {
		handle.InternalError(err)
	}

	fileName := j.Directory.GetDataFile()
	dataFile, err := os.Create(fileName)
	if err != nil {
		handle.InternalError(err)
	}
	defer dataFile.Close()

	data := bufio.NewWriter(dataFile)
	writeTitle(data, j)
	data.WriteString("Timestamp, CPU utilization %, Memory Usage Mb, Disk Write Kb, Outbound Network Traffic Kb\n")

	trafficDriverFinished := make(chan bool)
	quitChan := make(chan bool)
	go watchContainer(cli, driverID, j.ExpectedRunTime+20*time.Second, trafficDriverFinished, quitChan)

	if j.SummaryStatisticsData {
		j.collectSummaryStatisticsData(data, cli, appID, trafficDriverFinished, quitChan)
	} else {
		j.collectTimeseriesData(data, cli, appID, trafficDriverFinished, quitChan)
	}

	log.Debug().Msgf("writing captued data to file: %s...", dataFile.Name())
	err = data.Flush()
	if err != nil {
		handle.InternalError(err)
	}

	log.Debug().Msgf("done monitoring job %s", j.Name)

	timeout := time.Millisecond * 300
	cli.ContainerStop(context.Background(), appID, &timeout)
}

func watchContainer(client *client.Client, containerID string, timeout time.Duration, finishedWatching chan bool, quitChan chan bool) {
	log.Debug().Msgf("waiting for container %s to exit...", containerID)
	bC, _ := client.ContainerWait(context.Background(), containerID, container.WaitConditionNotRunning)
	select {
	case <-bC:
		log.Debug().Msg("driver container stopped. Writing to finishedWatching chan...")
		finishedWatching <- true
		return
	case <-quitChan:
		log.Debug().Msgf("quit signal received. Stopped watching container %s", containerID)
		return
	}
}

type statSnapshot struct {
	Tx, CPU, System float64
}

func (j *Job) collectTimeseriesData(data *bufio.Writer, cli *client.Client, appID string, trafficDriverFinished chan bool, quit chan bool) {
	previous := statSnapshot{}
	ticker := time.NewTicker(j.DataCollectionInterval)
	for {
		select {
		case <-ticker.C:
			stats := getStats(cli, appID)
			previous = writeData(data, &stats, &previous)
		case <-trafficDriverFinished:
			log.Debug().Msg("recieved message that traffic driver has stopped")
			return
		case <-time.After(j.LoadDuration):
			log.Debug().Msg("timeout reached, sending quit signal to watcher...")
			quit <- true
			return
		}
	}
}

// data is random and only collected during periods of application load
func (j *Job) collectSummaryStatisticsData(data *bufio.Writer, cli *client.Client, appID string, trafficDriverFinished chan bool, quit chan bool) {
	previous := statSnapshot{}

	// wait 5 seconds to avoid utilization spikes due to surge of traffic
	collectionDelay := 5 * time.Second
	log.Debug().Msgf("waiting %s to avoid usage spikes caused by a surge in traffic...", collectionDelay.String())
	time.Sleep(j.LoadDelay + collectionDelay)

	log.Debug().Msgf("collecting summary statistics data randomly within a %s interval...", j.DataCollectionInterval.String())
	// stop collecting 2 second before traffic stops being sent just to be defensive
	timeoutPeriod := j.LoadDuration - (collectionDelay + 3*time.Second)
	log.Debug().Msgf("this collection process will time out in %s...", timeoutPeriod.String())

	statsChan := make(chan types.StatsJSON, 1)
	go getStatsRandomlyWithinInterval(j.DataCollectionInterval, cli, statsChan, appID)
	for {
		select {
		case <-trafficDriverFinished:
			log.Debug().Msg("recieved message that traffic driver has stopped")
			return
		case <-time.After(timeoutPeriod):
			log.Debug().Msg("timeout reached, sending quit signal to watcher...")
			quit <- true
			return
		case stats := <-statsChan:
			previous = writeData(data, &stats, &previous)
			go getStatsRandomlyWithinInterval(j.DataCollectionInterval, cli, statsChan, appID)
		}
	}
}

func getStats(cli *client.Client, containerName string) types.StatsJSON {
	statsReader, err := cli.ContainerStatsOneShot(context.TODO(), containerName)
	if err != nil {
		handle.InternalError(err)
	}

	defer statsReader.Body.Close()

	stats := types.StatsJSON{}
	buf, err := io.ReadAll(statsReader.Body)
	if err != nil {
		handle.InternalError(err)
	}
	err = json.Unmarshal(buf, &stats)
	if err != nil {
		handle.InternalError(err)
	}

	return stats
}

func getStatsRandomlyWithinInterval(interval time.Duration, cli *client.Client, statsChan chan types.StatsJSON, appID string) {
	stats := getStats(cli, appID)
	statsChan <- stats

	var sleepMillis int
	if interval == time.Second {
		// Try to prevent more than two or three checks per second to avoid straining the system
		sleepMillis = rand.Intn(700) + 300
	} else {
		sleepMillis = rand.Intn(int(interval.Milliseconds()))
	}

	sleepTime := time.Duration(sleepMillis) * time.Millisecond
	time.Sleep(sleepTime)
}

func writeTitle(data *bufio.Writer, j *Job) {
	if j.SummaryStatisticsData {
		data.WriteString("Random Summary Statistics")
	} else {
		data.WriteString("Timeseries")
	}
	data.WriteString(" Data Measuring the Perfomance of Job ")
	data.WriteString(j.Name)
	data.WriteString("\n")
}

func writeData(data *bufio.Writer, stats *types.StatsJSON, previous *statSnapshot) statSnapshot {
	cpuPercent := calculateCPUPercentUnix(previous.CPU, previous.System, stats)
	previousCPU := float64(stats.CPUStats.CPUUsage.TotalUsage)
	previousSystem := float64(stats.CPUStats.SystemUsage)
	_, tx := calculateNetwork(stats.Networks)
	txDiff := tx - previous.Tx
	previousTx := tx

	data.WriteString(time.Now().String())
	data.WriteByte(',')
	data.WriteString(fmt.Sprintf("%.3f,", cpuPercent))
	data.WriteString(fmt.Sprintf("%.3f,", (float64(stats.MemoryStats.Usage)/1024)/1024))
	data.WriteString(fmt.Sprintf("%.3f,", float64(stats.StorageStats.WriteSizeBytes)/1024))
	data.WriteString(fmt.Sprintf("%.3f", txDiff/1024))
	data.WriteString("\n")

	return statSnapshot{
		previousTx, previousCPU, previousSystem,
	}
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
