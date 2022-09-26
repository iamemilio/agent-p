package app

import (
	"agent-p/handle"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	defaultDriverImage = "quay.io/emiliogarcia_1/traffic-driver:latest"
	version            = "0.2.0"
)

type RunConfig struct {
	Version            string `yaml:"version"`
	Server             string `yaml:"new-relic-server"` // production, staging, eu
	LicenseKey         string `yaml:"new-relic-license-key,omitempty"`
	CollectionEndpoint string `yaml:",omitempty"` // New Relic Collection Endpoint
	Runs               []Run  `yaml:"jobs"`
}

type Run struct {
	Name          string `yaml:"name"`
	Data          `yaml:"data"`
	App           `yaml:"app"`
	TrafficDriver `yaml:"traffic-driver"`
}

type App struct {
	Image   string            `yaml:"image"`
	Port    *uint             `yaml:"service-port"`
	EnvVars map[string]string `yaml:"environment-variables"`
}

type Data struct {
	SummaryStatistic bool   `yaml:"summary-statistics"`
	Interval         string `yaml:"collection-interval"`
}

type TrafficDriver struct {
	Endpoint string `yaml:"service-endpoint"`
	Image    string `yaml:"image"`
	Delay    string `yaml:"startup-delay"`
	Traffic  `yaml:"traffic"`
}

type Traffic struct {
	Duration string `yaml:"duration"`
	Rate     *uint  `yaml:"requests-per-second"`
	Users    *uint  `yaml:"concurrent-requests"`
}

var (
	errNameEmpty          = errors.New("run.name can not be empty")
	errNoLicenseKey       = errors.New("a New Relic license key must be provided, either set the new-relic-license-key field in the config.yaml file or set the environment variable \"NEW_RELIC_LICENSE_KEY\"")
	errNoRuns             = errors.New("config error: run config must have at least one run")
	errServerNotSupported = errors.New("config error: new-relic-server must be either: production, stagin, eu")
	serverEndpoints       = map[string]string{
		"production": "",
		"staging":    "staging-collector.newrelic.com",
		"eu":         "",
	}
)

// Defaults
const (
	delay       = "20s"
	duration    = "3m"
	rate        = 100
	users       = 3
	intervalStr = "1s"
	interval    = 1
)

func (r *RunConfig) defaultAndValidate() error {
	if len(r.Runs) == 0 {
		return errNoRuns
	}

	endpoint, ok := serverEndpoints[strings.TrimSpace(strings.ToLower(r.Server))]
	if !ok {
		return errServerNotSupported
	}

	r.CollectionEndpoint = endpoint

	if r.LicenseKey == "" {
		key := os.Getenv("NEW_RELIC_LICENSE_KEY")
		if key == "" {
			return errNoLicenseKey
		}
		r.LicenseKey = key
	}

	seen := map[string]bool{}
	for i := 0; i < len(r.Runs); i++ {
		run := &r.Runs[i]
		if seen[run.Name] {
			return fmt.Errorf("job name %s already in use, please give each job a unique name", run.Name)
		} else {
			seen[run.Name] = true
		}

		err := run.defaultAndValidate()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Run) defaultAndValidate() error {
	if r.Name == "" {
		return errNameEmpty
	}

	err := r.App.defaultAndValidate()
	if err != nil {
		return err
	}

	err = r.Data.defaultAndValidate()
	if err != nil {
		return err
	}

	err = r.TrafficDriver.defaultAndValidate()
	if err != nil {
		return err
	}

	return nil
}

func (d *Data) defaultAndValidate() error {
	if d.Interval == "" {
		d.Interval = intervalStr
	} else {
		interval, err := validateDuration(d.Interval)
		if err != nil {
			return err
		}

		d.Interval = interval
	}

	return nil
}

var (
	errImageEmpty = errors.New("config error: run.app.image must be a valid container image")
)

func (a *App) defaultAndValidate() error {
	if a.Image == "" {
		return errImageEmpty
	}
	return nil
}

func (t *TrafficDriver) defaultAndValidate() error {
	if t.Endpoint == "" {
		t.Endpoint = "/"
	}
	if t.Image == "" {
		t.Image = defaultDriverImage
	}
	if t.Delay == "" {
		t.Delay = delay
	} else {
		duration, err := validateDuration(t.Delay)
		if err != nil {
			return err
		}

		t.Delay = duration
	}
	return t.Traffic.defaultAndValidate()
}

func (t *Traffic) defaultAndValidate() error {
	if t.Duration == "" {
		t.Duration = duration
	} else {
		duration, err := validateDuration(t.Duration)
		if err != nil {
			return err
		}

		t.Duration = duration
	}
	if t.Rate == nil {
		t.Rate = UintPointer(rate)
	}
	if t.Users == nil {
		t.Users = UintPointer(users)
	}

	return nil
}

func UintPointer(val int) *uint {
	a := uint(val)
	return &a
}

func CreateConfig(file string) {
	config, err := defaultConfig()
	if err != nil {
		handle.InternalError(err)
	}

	os.WriteFile(file, config, 0664)
}

// defaultConfig returns a boiler plate config file with
func defaultConfig() ([]byte, error) {
	config := RunConfig{
		Version: version,
		Server:  "production",
		Runs: []Run{
			{
				Name: "example",
				App: App{
					Image: "YOUR APP CONTAINER IMAGE",
					Port:  UintPointer(8000),
					EnvVars: map[string]string{
						"EXAMPLE_KEY": "EXAMPLE_VALUE",
					},
				},
				Data: Data{
					Interval: intervalStr,
				},
				TrafficDriver: TrafficDriver{
					Endpoint: "/your_http_endpoint",
					Image:    defaultDriverImage,
					Delay:    delay,
					Traffic: Traffic{
						Duration: duration,
						Rate:     UintPointer(rate),
						Users:    UintPointer(users),
					},
				},
			},
		},
	}

	return yaml.Marshal(config)
}

func (cfg *RunConfig) CreateJobs() Batch {
	log.Info().Msg("Creating Docker Compose workspaces for jobs...")
	jobs := make([]Job, len(cfg.Runs))
	workspace, err := CreateJobWorkspace(JobsDir)
	if err != nil {
		handle.InternalError(err)
	}

	for i, run := range cfg.Runs {
		log.Debug().Msgf("\ncreating resources for job \"%s\"", run.Name)
		job, compose := run.toJob(cfg.LicenseKey, cfg.CollectionEndpoint)
		jobs[i] = job
		jobDir, err := compose.WriteFile(job.Name, workspace)
		if err != nil {
			handle.InternalError(err)
		}

		jobs[i].Directory = jobDir
		log.Debug().Msg("job succesfully created!\n")
	}

	return jobs
}

const (
	appName    = "app"
	driverName = "driver"
)

// ToJob converts a run to a runnable job
func (run *Run) toJob(licenseKey, endpoint string) (Job, DockerCompose) {
	// Get Durations
	collectionInterval, err := parseDuration(run.Data.Interval)
	if err != nil {
		handle.InternalError(err)
	}

	trafficDuration, err := parseDuration(run.TrafficDriver.Traffic.Duration)
	if err != nil {
		handle.InternalError(err)
	}

	trafficDelay, err := parseDuration(run.TrafficDriver.Delay)
	if err != nil {
		handle.InternalError(err)
	}

	if trafficDuration.Seconds() < 20 && run.Data.SummaryStatistic {
		handle.IncorrectUsage(errors.New("please use a `traffic-driver.traffic.duration` greater than 20 seconds for summary statistic jobs, otherwise data will likely be inaccurate"))
	}

	if collectionInterval >= trafficDuration {
		handle.IncorrectUsage(fmt.Errorf("data collection interval %s for job %s can not be greater than or equal to the total experiment duration %s", collectionInterval.String(), run.Name, trafficDuration.String()))
	}

	if collectionInterval > 10*time.Second || collectionInterval < 500*time.Millisecond {
		handle.IncorrectUsage(fmt.Errorf("data collected for job %s will not be accurate when collected at an interval of %s. Keep the collection interval between 500 milliseconds and 100 seconds", run.Name, collectionInterval.String()))
	}

	// Create Docker Compose Object
	compose := DockerCompose{
		Version:  composeVersion,
		Services: map[string]Service{},
	}

	compose.Services[appName] = Service{
		Image:       run.App.Image,
		Ports:       []string{},
		Environment: run.appEnv(licenseKey, endpoint),
	}

	compose.Services[driverName] = Service{
		Image:       run.TrafficDriver.Image,
		Environment: run.driverEnv(appName, int(trafficDelay.Seconds())),
		DependsOn: []string{
			appName,
		},
	}

	return Job{
		Name:                   run.Name,
		SummaryStatisticsData:  run.SummaryStatistic,
		DataCollectionInterval: collectionInterval,
		ExpectedRunTime:        trafficDuration + trafficDelay,
		LoadDuration:           trafficDuration,
		LoadDelay:              trafficDelay,
	}, compose
}

func toComposeEnvVar(vars map[string]string) []string {
	out := []string{}
	for k, v := range vars {
		if k != "" {
			out = append(out, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return out
}

func (run *Run) appEnv(licenseKey, endpoint string) []string {
	vars := []string{
		fmt.Sprintf("%s=%s", "NEW_RELIC_LICENSE_KEY", licenseKey),
		fmt.Sprintf("%s=%s", "NEW_RELIC_APP_NAME", run.Name),
	}

	if endpoint != "" {
		vars = append(vars, fmt.Sprintf("%s=%s", "NEW_RELIC_HOST", endpoint))
	}

	vars = append(vars, toComposeEnvVar(run.App.EnvVars)...)
	return vars
}

func (run *Run) driverEnv(appName string, delay int) []string {
	vars := []string{
		fmt.Sprintf("%s=%s", "APP_NAME", appName),
		fmt.Sprintf("%s=%d", "TRAFFIC_DRIVER_DELAY", delay),
		fmt.Sprintf("%s=%d", "SERVICE_PORT", *run.App.Port),
		fmt.Sprintf("%s=%s", "SERVICE_ENDPOINT", run.TrafficDriver.Endpoint),
		fmt.Sprintf("%s=%d", "CONCURRENT_REQUESTS", *run.TrafficDriver.Traffic.Users),
		fmt.Sprintf("%s=%d", "REQUESTS_PER_SECOND", *run.TrafficDriver.Traffic.Rate),
		fmt.Sprintf("%s=%s", "DURATION", run.TrafficDriver.Traffic.Duration),
	}
	return vars
}

// GetConfig reads, unmarshals, and vaildates a RunConfig
func GetConfig(file string) *RunConfig {
	cfgBytes, err := os.ReadFile(file)
	if err != nil {
		handle.InternalError(err)
	}

	cfg := RunConfig{}
	err = yaml.Unmarshal(cfgBytes, &cfg)
	if err != nil {
		handle.InternalError(err)
	}

	err = cfg.defaultAndValidate()
	if err != nil {
		handle.IncorrectUsage(err)
	}
	return &cfg
}

func validateDuration(duration string) (string, error) {
	clean := strings.TrimSpace(strings.ToLower(duration))
	if regexp.MustCompile(`^(\d+)([s|m])$`).MatchString(clean) {
		return clean, nil
	}
	return "", fmt.Errorf("collection-interval must have both a number and duration unit: examples: 3m or 1s. Duration \"%s\" is invalid", clean)
}

func parseDuration(duration string) (time.Duration, error) {
	if strings.Contains(duration, "s") {
		duration, err := strconv.Atoi(strings.Split(duration, "s")[0])
		if err != nil {
			return -1 * time.Second, err
		}

		return time.Duration(duration) * time.Second, nil
	} else {
		duration, err := strconv.Atoi(strings.Split(duration, "m")[0])
		if err != nil {
			return -1 * time.Second, err
		}

		return time.Duration(duration) * time.Minute, nil
	}
}
