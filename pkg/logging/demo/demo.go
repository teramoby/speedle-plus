//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	logging "github.com/teramoby/speedle-plus/pkg/logging"
)

type Config struct {
	LogConfig *logging.LogConfig
}

func main() {
	err := initLog()
	if err != nil {
		panic(err)
	}

	// Call the Engine's native slog logger; library initialization leaves the
	// host application's slog.Default and standard log output untouched.
	logger := logging.Default()
	logger.Debug("This is a debug log entry")
	logger.Info("This is an info log entry")
	logger.Warn("This is a warning log entry")
	logger.Error("This is an error log entry")
	logger.Info("A group of walrus emerges from the ocean", "animal", "walrus", "size", 10)
	contextLogger := logger.With("common", "this is a common field", "other", "logged always")
	contextLogger.Info("I'll be logged with common and other fields")
	contextLogger.Info("Me too")
	// logging.Fatal("The ice breaks!") logs then exits with status 1.
	// logging.Panic("The ice breaks!") logs then panics.

}

func initLog() error {
	raw, err := os.ReadFile("./config.json")
	if err != nil {
		return fmt.Errorf("failed to load the config file, err: %v", err)
	}

	var config Config
	err = json.Unmarshal(raw, &config)
	if err != nil {
		return fmt.Errorf("failed to parse the config file, err: %v", err)
	}

	if config.LogConfig != nil {
		err = logging.InitLog(config.LogConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize the log library, err: %v", err)
		}
	} else {
		return fmt.Errorf("no log configurations")
	}

	return nil
}
