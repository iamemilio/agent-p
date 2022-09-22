package app

import (
	"os"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

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

// WriteFile writes a docker compose file to your local disk
func (compose *DockerCompose) WriteFile(name, workspace string) (JobDirectory, error) {
	yaml, err := yaml.Marshal(compose)
	if err != nil {
		return "", err
	}

	// Make directory for job
	jobDir, err := CreateJobDirectory(workspace, name)
	if err != nil {
		return "", err
	}

	log.Debug().Msgf("created directory %s", jobDir)

	// Overwrite files that already exist
	composeFile := jobDir.GetCompose()
	f, err := os.Create(composeFile)
	if err != nil {
		return "", err
	}

	log.Debug().Msgf("created docker compose file: %s", composeFile)
	log.Debug().Msg("writing marshalled content to compose file")

	_, err = f.Write(yaml)
	if err != nil {
		return "", err
	}

	f.Close()

	log.Debug().Msgf("content written to compose file successfully")
	return jobDir, nil
}
