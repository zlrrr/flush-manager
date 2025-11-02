package process

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_Start(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid command",
			command: "sleep",
			args:    []string{"0.1"},
			wantErr: false,
		},
		{
			name:    "invalid command",
			command: "/nonexistent/command",
			args:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(tt.command, tt.args)
			ctx := context.Background()

			err := m.Start(ctx)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Clean up
				_ = m.Stop(1 * time.Second)
			}
		})
	}
}

func TestManager_Wait(t *testing.T) {
	t.Run("process exits normally", func(t *testing.T) {
		m := NewManager("sh", []string{"-c", "exit 0"})
		ctx := context.Background()

		err := m.Start(ctx)
		require.NoError(t, err)

		reason, err := m.Wait()
		assert.Equal(t, ExitReasonAbnormal, reason)
		assert.NoError(t, err)
	})

	t.Run("process exits with error", func(t *testing.T) {
		m := NewManager("sh", []string{"-c", "exit 1"})
		ctx := context.Background()

		err := m.Start(ctx)
		require.NoError(t, err)

		reason, err := m.Wait()
		assert.Equal(t, ExitReasonAbnormal, reason)
		assert.Error(t, err)
	})
}

func TestManager_Stop(t *testing.T) {
	t.Run("stop running process", func(t *testing.T) {
		m := NewManager("sleep", []string{"10"})
		ctx := context.Background()

		err := m.Start(ctx)
		require.NoError(t, err)

		// Give process time to start
		time.Sleep(100 * time.Millisecond)

		err = m.Stop(5 * time.Second)
		assert.NoError(t, err)
	})

	t.Run("stop already stopped process", func(t *testing.T) {
		m := NewManager("sh", []string{"-c", "exit 0"})
		ctx := context.Background()

		err := m.Start(ctx)
		require.NoError(t, err)

		// Wait for process to exit
		_, _ = m.Wait()

		err = m.Stop(1 * time.Second)
		assert.NoError(t, err)
	})

	t.Run("force kill on timeout", func(t *testing.T) {
		// Create a process that ignores SIGTERM
		m := NewManager("sh", []string{"-c", "trap '' TERM; sleep 10"})
		ctx := context.Background()

		err := m.Start(ctx)
		require.NoError(t, err)

		// Give process time to setup trap
		time.Sleep(100 * time.Millisecond)

		// Stop with very short timeout to force kill
		err = m.Stop(100 * time.Millisecond)
		assert.NoError(t, err)
	})
}

func TestManager_Restart(t *testing.T) {
	t.Run("restart process successfully", func(t *testing.T) {
		m := NewManager("sleep", []string{"10"})
		ctx := context.Background()

		// Start initial process
		err := m.Start(ctx)
		require.NoError(t, err)

		// Get PID of first process
		mgr := m.(*manager)
		firstPID := mgr.cmd.Process.Pid

		// Restart
		err = m.Restart(ctx)
		assert.NoError(t, err)

		// Get PID of second process
		secondPID := mgr.cmd.Process.Pid

		// PIDs should be different
		assert.NotEqual(t, firstPID, secondPID)

		// Clean up
		_ = m.Stop(1 * time.Second)
	})

	t.Run("restart marks old process exit as restart", func(t *testing.T) {
		m := NewManager("sleep", []string{"10"})
		ctx := context.Background()

		err := m.Start(ctx)
		require.NoError(t, err)

		// Give process time to start
		time.Sleep(100 * time.Millisecond)

		// Start restart in background
		restartDone := make(chan error, 1)
		go func() {
			restartDone <- m.Restart(ctx)
		}()

		// Wait for old process to exit during restart
		reason, _ := m.Wait()

		// The old process exit should be marked as restart
		assert.Equal(t, ExitReasonRestart, reason)

		// Ensure restart completes
		err = <-restartDone
		assert.NoError(t, err)

		// Clean up
		_ = m.Stop(1 * time.Second)
	})
}

func TestManager_ContextCancellation(t *testing.T) {
	t.Run("context cancellation stops process", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := NewManager("sleep", []string{"10"})

		err := m.Start(ctx)
		require.NoError(t, err)

		// Cancel context
		cancel()

		// Process should exit
		done := make(chan struct{})
		go func() {
			m.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success - process exited
		case <-time.After(2 * time.Second):
			t.Fatal("process did not exit after context cancellation")
		}
	})
}

func TestNewManager(t *testing.T) {
	t.Run("create manager with valid params", func(t *testing.T) {
		m := NewManager("echo", []string{"hello"})
		assert.NotNil(t, m)
	})

	t.Run("create manager with no args", func(t *testing.T) {
		m := NewManager("ls", nil)
		assert.NotNil(t, m)
	})
}

func TestExitReason(t *testing.T) {
	// Test that ExitReason constants have expected values
	assert.Equal(t, ExitReason(0), ExitReasonUnknown)
	assert.Equal(t, ExitReason(1), ExitReasonAbnormal)
	assert.Equal(t, ExitReason(2), ExitReasonRestart)
}

// BenchmarkManager_StartStop benchmarks the start/stop cycle
func BenchmarkManager_StartStop(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		m := NewManager("sh", []string{"-c", "exit 0"})
		_ = m.Start(ctx)
		_, _ = m.Wait()
	}
}
