package main

import (
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

var (
	logger *log.Logger
	once   sync.Once
)

func GetLogger() *log.Logger {
	once.Do(func() {
		logger = log.NewWithOptions(os.Stderr, log.Options{
			ReportCaller:    true,
			ReportTimestamp: true,
			TimeFormat:      time.Kitchen,
		})
	})
	return logger
}
