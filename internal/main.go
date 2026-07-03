package main

import "github.com/akmadian/alexandria/internal/importer"

func main() {
	logger := GetLogger()
	logger.Info("Main application started")

	importer.Run("../tst/data")

	logger.Info("Main application finished")
}
