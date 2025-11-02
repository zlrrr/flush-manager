package manager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zlrrr/flush-manager/internal/logger"
	"github.com/zlrrr/flush-manager/internal/process"
	"github.com/zlrrr/flush-manager/internal/watcher"
)

// Config holds the configuration for the manager
type Config struct {
	Command        string
	Args           []string
	ConfigFilePath string
}

// Manager is the main manager that coordinates process and file watching
type Manager struct {
	config         Config
	processManager process.Manager
	fileWatcher    watcher.FileWatcher
	ctx            context.Context
	cancel         context.CancelFunc
}

// New creates a new Manager instance
func New(config Config) (*Manager, error) {
	logger.Info("Initializing manager with command: %s", config.Command)

	if config.Command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	ctx, cancel := context.WithCancel(context.Background())

	pm := process.NewManager(config.Command, config.Args)

	// Create file watcher if config file is specified
	fw, err := watcher.NewFileWatcher(config.ConfigFilePath)
	if err != nil {
		cancel()
		logger.Error("Failed to create file watcher: %v", err)
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	m := &Manager{
		config:         config,
		processManager: pm,
		fileWatcher:    fw,
		ctx:            ctx,
		cancel:         cancel,
	}

	logger.Info("Manager initialized successfully")
	return m, nil
}

// Run starts the manager and blocks until it should exit
func (m *Manager) Run() error {
	logger.Info("Starting manager run loop...")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	logger.Debug("Signal handlers registered for SIGINT and SIGTERM")

	// Start the child process
	if err := m.processManager.Start(m.ctx); err != nil {
		logger.Error("Failed to start child process: %v", err)
		return fmt.Errorf("failed to start child process: %w", err)
	}

	logger.Info("Manager started, child process: %s", m.config.Command)

	// Start file watcher
	if err := m.fileWatcher.Start(m.ctx); err != nil {
		logger.Error("Failed to start file watcher: %v", err)
		return fmt.Errorf("failed to start file watcher: %w", err)
	}
	if m.config.ConfigFilePath != "" {
		logger.Info("Watching config file: %s", m.config.ConfigFilePath)
	}

	// Monitor process exit in background
	type exitResult struct {
		reason process.ExitReason
		err    error
	}
	exitChan := make(chan exitResult, 1)
	go func() {
		reason, err := m.processManager.Wait()
		exitChan <- exitResult{reason: reason, err: err}
	}()

	logger.Info("Entering main event loop")

	// Main event loop
	for {
		select {
		case sig := <-sigChan:
			logger.Info("Received signal: %v, shutting down gracefully...", sig)
			return m.shutdown()

		case <-m.fileWatcher.Changes():
			logger.Info("Config file change detected, restarting child process...")
			if err := m.processManager.Restart(m.ctx); err != nil {
				logger.Error("Failed to restart process: %v", err)
				return err
			}
			logger.Info("Child process restarted successfully after config change")

			// Restart the exit monitor goroutine
			go func() {
				reason, err := m.processManager.Wait()
				exitChan <- exitResult{reason: reason, err: err}
			}()

		case result := <-exitChan:
			// If process was restarted by us, continue
			if result.reason == process.ExitReasonRestart {
				logger.Debug("Process exit was due to restart, continuing...")
				continue
			}

			// If process exited abnormally, manager should exit too
			if result.err != nil {
				logger.Error("Child process exited with error: %v", result.err)
			} else {
				logger.Info("Child process exited normally")
			}
			return m.shutdown()

		case <-m.ctx.Done():
			logger.Debug("Context cancelled, shutting down...")
			return m.shutdown()
		}
	}
}

// shutdown performs graceful shutdown
func (m *Manager) shutdown() error {
	logger.Info("Shutting down manager...")

	// Cancel context to stop watchers
	m.cancel()
	logger.Debug("Context cancelled")

	// Close file watcher
	if err := m.fileWatcher.Close(); err != nil {
		logger.Error("Error closing file watcher: %v", err)
	} else {
		logger.Debug("File watcher closed")
	}

	// Stop child process gracefully
	if err := m.processManager.Stop(10 * time.Second); err != nil {
		logger.Error("Error stopping child process: %v", err)
		return err
	}

	logger.Info("Manager shutdown complete")
	return nil
}
