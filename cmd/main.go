package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/stevebriskin/microtemp"

	"github.com/edaniels/golog"
)

const ITERATIONS = 100

func main() {
	logger := golog.NewDevelopmentLogger("client")
	ctx := context.Background()

	var conf microtemp.Config
	err := ParseConfig(&conf)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Infof("Loaded config file with secrets: %+v", conf)

	err = microtemp.Initialize(ctx, conf, logger)
	if err != nil {
		logger.Fatal(err)
	}
	defer microtemp.Close()

	for i := 0; i < ITERATIONS; i++ {
		logger.Infof("Reading number %v.", i)
		microtemp.DoAll(ctx, logger)

		// inefficient: only starts sleep after last machine is done, not per machine. push logic down.
		logger.Info("Sleeping...")
		time.Sleep(time.Duration(conf.SleepTime) * time.Second)
	}
}

func ParseConfig(conf *microtemp.Config) error {
	fileName := os.Getenv("CONFIG")
	if fileName == "" {
		fileName = filepath.Join(os.Getenv("HOME"), ".viam", "temperatureconfig")
	}

	configBytes, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(configBytes, conf); err != nil {
		return err
	}

	return nil
}
