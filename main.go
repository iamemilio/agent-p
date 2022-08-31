package main

import (
	"agent-p/pkg/app"
	"agent-p/pkg/cmd"
	"agent-p/pkg/handle"
	"errors"

	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Setup  App
	log.Logger = zerolog.New(os.Stdout).Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: zerolog.TimeFormatUnix,
		PartsExclude: []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
		},
	}).Level(zerolog.InfoLevel)

	inputs := cmd.Execute()
	if inputs.ShouldExit {
		os.Exit(0)
	}

	if inputs.Silent && inputs.Debug {
		handle.IncorrectUsage(errors.New("application logs can not be both silent and printing debug logs"))
	}

	if inputs.Silent {
		log.Logger = log.Level(zerolog.Disabled)
	} else if inputs.Debug {
		log.Logger = log.Level(zerolog.DebugLevel)
	}

	log.Debug().Msgf("Inputs: %+v", inputs)

	// Main Body
	if inputs.Create != nil {
		configFile := inputs.Create.Config
		if inputs.Create.Jobs {
			log.Debug().Msg("creating jobs...")
			config := app.GetConfig(configFile)
			config.CreateJobs()
		} else {
			log.Debug().Msg("creating a config...")
			app.CreateConfig(configFile)
		}
	}
	if inputs.Clean != nil {
		log.Info().Msgf("Cleaning up jobs")
		config := app.GetConfig(inputs.Clean.Config)
		config.Clean()
	}
	if inputs.Run != nil {
		log.Debug().Msgf("running from config \"%s\"...", inputs.Run.Config)
		config := app.GetConfig(inputs.Run.Config)
		jobs := config.CreateJobs()
		jobs.Run(inputs.CleanRun)
	}
	os.Exit(0)
}
