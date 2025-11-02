package main

import (
	"flag"
	"fmt"
	"os"

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

	// Get additional args to pass to the child process
	args := flag.Args()

	config := manager.Config{
		Command:        *command,
		Args:           args,
		ConfigFilePath: *configFile,
	}

	m, err := manager.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		os.Exit(1)
	}

	if err := m.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Manager error: %v\n", err)
		os.Exit(1)
	}
}
