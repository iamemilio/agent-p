package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

type JobDirectory string

func CreateJobWorkspace(workspaceName string) (string, error) {
	return mkdirIfNotExists("./", workspaceName)
}

// only works with jobs in the current working directory
func ToLocalJobDirectory(jobName string) JobDirectory {
	return JobDirectory("./" + JobsDir + "/" + strings.ReplaceAll(jobName, " ", "-") + "/")
}

func CreateJobDirectory(path, name string) (JobDirectory, error) {
	filename, err := mkdirIfNotExists(path, strings.ReplaceAll(name, " ", "-"))
	return JobDirectory(filename), err
}

func (jd JobDirectory) GetCompose() string {
	return fmt.Sprintf("%sdocker-compose.yaml", jd)
}

func (jd JobDirectory) GetDataFile() string {
	return fmt.Sprintf("%sdata.csv", jd)
}

func mkdirIfNotExists(path, name string) (string, error) {
	path = strings.TrimSpace(path)
	if path[len(path)-1] != '/' {
		return "", fmt.Errorf("mkdirIfNotExists() error: path %s must end with a '/' character", path)
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	longFileName := fmt.Sprintf("%s%s", path, name)
	for _, file := range files {
		if file.Name() == name && file.IsDir() {
			log.Debug().Msgf("file \"%s\" exists, and will not be created...", longFileName)
			return longFileName + "/", nil
		}
	}

	log.Debug().Msgf("creating file \"%s\"...", longFileName)
	return longFileName + "/", os.Mkdir(longFileName, os.ModePerm)
}

func (jd JobDirectory) ReadData() {
	//TODO
}
