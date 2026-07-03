package main

import (
	"os"
	"time"

	"github.com/akmadian/alexandria/internal/importer"
	"github.com/charmbracelet/log"
)

func main() {
	// Configure the one shared logger and register it as charm's default, so any
	// package can log via the package-level log.Info/Warn/Error functions.
	log.SetDefault(log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
	}))

	log.Info("Main application started")
	importer.Run("../testdata")
	log.Info("Main application finished")
}
