package manager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	if config.Command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	ctx, cancel := context.WithCancel(context.Background())

	pm := process.NewManager(config.Command, config.Args)

	// Create file watcher if config file is specified
	fw, err := watcher.NewFileWatcher(config.ConfigFilePath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	m := &Manager{
		config:         config,
		processManager: pm,
		fileWatcher:    fw,
		ctx:            ctx,
		cancel:         cancel,
	}

	return m, nil
}

// Run starts the manager and blocks until it should exit
func (m *Manager) Run() error {
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the child process
	if err := m.processManager.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start child process: %w", err)
	}

	fmt.Printf("Manager started, child process: %s\n", m.config.Command)

	// Start file watcher
	if err := m.fileWatcher.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start file watcher: %w", err)
	}
	if m.config.ConfigFilePath != "" {
		fmt.Printf("Watching config file: %s\n", m.config.ConfigFilePath)
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

	// Main event loop
	for {
		select {
		case sig := <-sigChan:
			fmt.Printf("Received signal: %v, shutting down gracefully...\n", sig)
			return m.shutdown()

		case <-m.fileWatcher.Changes():
			fmt.Println("Config file changed, restarting child process...")
			if err := m.processManager.Restart(m.ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to restart process: %v\n", err)
				return err
			}
			fmt.Println("Child process restarted successfully")

			// Restart the exit monitor goroutine
			go func() {
				reason, err := m.processManager.Wait()
				exitChan <- exitResult{reason: reason, err: err}
			}()

		case result := <-exitChan:
			// If process was restarted by us, continue
			if result.reason == process.ExitReasonRestart {
				continue
			}

			// If process exited abnormally, manager should exit too
			if result.err != nil {
				fmt.Fprintf(os.Stderr, "Child process exited with error: %v\n", result.err)
			} else {
				fmt.Println("Child process exited normally")
			}
			return m.shutdown()

		case <-m.ctx.Done():
			return m.shutdown()
		}
	}
}

// shutdown performs graceful shutdown
func (m *Manager) shutdown() error {
	fmt.Println("Shutting down manager...")

	// Cancel context to stop watchers
	m.cancel()

	// Close file watcher
	if err := m.fileWatcher.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Error closing file watcher: %v\n", err)
	}

	// Stop child process gracefully
	if err := m.processManager.Stop(10 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping child process: %v\n", err)
		return err
	}

	fmt.Println("Manager shutdown complete")
	return nil
}
