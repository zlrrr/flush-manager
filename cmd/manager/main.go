package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zlrrr/flush-manager/internal/logger"
	"github.com/zlrrr/flush-manager/internal/manager"
)

const (
	defaultCommand    = "/usr/local/bin/redis-exporter"
	defaultConfigFile = "/usr/local/bin/conf/exporter.conf"
)

var (
	command    = flag.String("command", defaultCommand, "Command to execute")
	configFile = flag.String("config", defaultConfigFile, "Config file to watch for changes")
	version    = flag.Bool("version", false, "Print version information")
)

const Version = "1.0.0"

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("flush-manager version %s\n", Version)
		os.Exit(0)
	}

	logger.Info("=== Flush Manager v%s starting ===", Version)
	logger.Info("PID: %d", os.Getpid())

	// Get additional args to pass to the child process
	args := flag.Args()

	config := manager.Config{
		Command:        *command,
		Args:           args,
		ConfigFilePath: *configFile,
	}

	logger.Info("Configuration: command=%s, config_file=%s, args=%v", *command, *configFile, args)

	m, err := manager.New(config)
	if err != nil {
		logger.Fatal("Failed to create manager: %v", err)
	}

	if err := m.Run(); err != nil {
		logger.Fatal("Manager error: %v", err)
	}

	logger.Info("Manager exiting normally")
}
