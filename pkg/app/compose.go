package app

import (
	"agent-p/pkg/handle"
	"fmt"
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
