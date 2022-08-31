package handle

import (
	"os"

	"github.com/rs/zerolog/log"
)

func IncorrectUsage(err error) {
	log.Error().Msg(err.Error())
	os.Exit(2)
}

func InternalError(err error) {
	log.Error().Msg(err.Error())
	os.Exit(1)
}

func DockerComposeError(composeCmd string) {
	log.Info().Msgf("Docker compose exited with an error. To troubleshoot this, please run \"%s\".", composeCmd)
	os.Exit(1)
}
