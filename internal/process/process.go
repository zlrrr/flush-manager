package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/zlrrr/flush-manager/internal/logger"
)

// ExitReason represents why the process exited
type ExitReason int

const (
	ExitReasonUnknown ExitReason = iota
	ExitReasonAbnormal                    // Process crashed or exited unexpectedly
	ExitReasonRestart                     // Process was restarted by manager
)

// Manager handles the lifecycle of a child process
type Manager interface {
	Start(ctx context.Context) error
	Restart(ctx context.Context) error
	Wait() (ExitReason, error)
	Stop(timeout time.Duration) error
}

type manager struct {
	command     string
	args        []string
	cmd         *exec.Cmd
	exitChan    chan exitInfo
	restartFlag bool
}

type exitInfo struct {
	reason ExitReason
	err    error
}

// NewManager creates a new process manager
func NewManager(command string, args []string) Manager {
	return &manager{
		command:  command,
		args:     args,
		exitChan: make(chan exitInfo, 1),
	}
}

// Start starts the child process
func (m *manager) Start(ctx context.Context) error {
	logger.Info("Starting child process: %s %v", m.command, m.args)

	m.cmd = exec.CommandContext(ctx, m.command, m.args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr
	m.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group
	}

	if err := m.cmd.Start(); err != nil {
		logger.Error("Failed to start process: %v", err)
		return fmt.Errorf("failed to start process: %w", err)
	}

	logger.Info("Child process started with PID: %d", m.cmd.Process.Pid)

	// Monitor process exit
	go m.monitorProcess()

	return nil
}

// Restart gracefully restarts the child process
func (m *manager) Restart(ctx context.Context) error {
	logger.Info("Restarting child process...")
	m.restartFlag = true

	if err := m.Stop(10 * time.Second); err != nil {
		logger.Error("Failed to stop process during restart: %v", err)
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Wait a bit before restarting
	time.Sleep(100 * time.Millisecond)

	m.restartFlag = false
	logger.Info("Restarting child process after stop")
	return m.Start(ctx)
}

// Wait waits for the process to exit and returns the reason
func (m *manager) Wait() (ExitReason, error) {
	info := <-m.exitChan
	return info.reason, info.err
}

// Stop stops the child process gracefully
func (m *manager) Stop(timeout time.Duration) error {
	if m.cmd == nil || m.cmd.Process == nil {
		logger.Debug("No process to stop")
		return nil
	}

	pid := m.cmd.Process.Pid
	logger.Info("Stopping child process (PID: %d) with timeout: %v", pid, timeout)

	// Send SIGTERM for graceful shutdown
	if err := m.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		if err.Error() != "os: process already finished" {
			logger.Error("Failed to send SIGTERM to process: %v", err)
			return err
		}
		logger.Debug("Process already finished")
		return nil
	}

	logger.Debug("Sent SIGTERM to process (PID: %d), waiting for graceful shutdown...", pid)

	// Wait for process to exit gracefully
	done := make(chan error, 1)
	go func() {
		_, err := m.cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-done:
		logger.Info("Child process (PID: %d) stopped gracefully", pid)
		return nil
	case <-time.After(timeout):
		// Force kill if timeout
		logger.Info("Timeout waiting for graceful shutdown, sending SIGKILL to process (PID: %d)", pid)
		return m.cmd.Process.Kill()
	}
}

// monitorProcess monitors the process and sends exit info when it exits
func (m *manager) monitorProcess() {
	err := m.cmd.Wait()

	reason := ExitReasonAbnormal
	if m.restartFlag {
		reason = ExitReasonRestart
		logger.Debug("Process exited due to restart request")
	} else {
		if err != nil {
			logger.Info("Child process exited abnormally: %v", err)
		} else {
			logger.Info("Child process exited normally")
		}
	}

	m.exitChan <- exitInfo{
		reason: reason,
		err:    err,
	}
}
