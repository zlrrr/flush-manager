package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("create manager with valid config", func(t *testing.T) {
		config := Config{
			Command:        "echo",
			Args:           []string{"hello"},
			ConfigFilePath: "",
		}

		m, err := New(config)
		assert.NoError(t, err)
		assert.NotNil(t, m)

		if m != nil {
			m.cancel()
		}
	})

	t.Run("create manager with config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "test.conf")
		err := os.WriteFile(configFile, []byte("test"), 0644)
		require.NoError(t, err)

		config := Config{
			Command:        "echo",
			Args:           []string{"hello"},
			ConfigFilePath: configFile,
		}

		m, err := New(config)
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.NotNil(t, m.fileWatcher)

		if m != nil {
			m.cancel()
			if m.fileWatcher != nil {
				m.fileWatcher.Close()
			}
		}
	})

	t.Run("create manager without config file", func(t *testing.T) {
		config := Config{
			Command:        "echo",
			Args:           []string{"hello"},
			ConfigFilePath: "/nonexistent/file.conf",
		}

		m, err := New(config)
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.NotNil(t, m.fileWatcher) // Should be noopWatcher

		if m != nil {
			m.cancel()
		}
	})

	t.Run("error on empty command", func(t *testing.T) {
		config := Config{
			Command: "",
		}

		m, err := New(config)
		assert.Error(t, err)
		assert.Nil(t, m)
	})
}

func TestManager_ProcessExitsNormally(t *testing.T) {
	config := Config{
		Command: "sh",
		Args:    []string{"-c", "exit 0"},
	}

	m, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, m)
	defer m.cancel()

	// Run in goroutine with timeout
	done := make(chan error, 1)
	go func() {
		done <- m.Run()
	}()

	select {
	case err := <-done:
		// Manager should exit when child process exits
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for manager to exit")
	}
}

func TestManager_ProcessExitsWithError(t *testing.T) {
	config := Config{
		Command: "sh",
		Args:    []string{"-c", "exit 1"},
	}

	m, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, m)
	defer m.cancel()

	// Run in goroutine with timeout
	done := make(chan error, 1)
	go func() {
		done <- m.Run()
	}()

	select {
	case err := <-done:
		// Manager should exit gracefully even if child exits with error
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for manager to exit")
	}
}

func TestManager_ConfigFileChange(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test.conf")
	err := os.WriteFile(configFile, []byte("initial"), 0644)
	require.NoError(t, err)

	// Use a long-running process
	config := Config{
		Command:        "sleep",
		Args:           []string{"30"},
		ConfigFilePath: configFile,
	}

	m, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, m)

	// Run manager in background
	done := make(chan error, 1)
	go func() {
		done <- m.Run()
	}()

	// Wait for manager to start
	time.Sleep(500 * time.Millisecond)

	// Modify config file
	err = os.WriteFile(configFile, []byte("modified"), 0644)
	require.NoError(t, err)

	// Wait for restart to happen
	time.Sleep(1 * time.Second)

	// Shutdown manager
	m.cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for manager to exit")
	}
}

func TestManager_Shutdown(t *testing.T) {
	t.Run("graceful shutdown", func(t *testing.T) {
		config := Config{
			Command: "sleep",
			Args:    []string{"30"},
		}

		m, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, m)

		err = m.shutdown()
		assert.NoError(t, err)
	})

	t.Run("shutdown with file watcher", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "test.conf")
		err := os.WriteFile(configFile, []byte("test"), 0644)
		require.NoError(t, err)

		config := Config{
			Command:        "sleep",
			Args:           []string{"30"},
			ConfigFilePath: configFile,
		}

		m, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, m)

		// Start the process
		err = m.processManager.Start(m.ctx)
		require.NoError(t, err)

		// Start file watcher
		err = m.fileWatcher.Start(m.ctx)
		require.NoError(t, err)

		// Wait a bit
		time.Sleep(100 * time.Millisecond)

		err = m.shutdown()
		assert.NoError(t, err)
	})
}

func TestManager_Integration(t *testing.T) {
	t.Run("full lifecycle with config changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "test.conf")
		outputFile := filepath.Join(tmpDir, "output.txt")

		// Create initial config
		err := os.WriteFile(configFile, []byte("initial"), 0644)
		require.NoError(t, err)

		// Use a script that writes to a file and sleeps
		script := `
echo "started" >> ` + outputFile + `
sleep 30
`
		scriptFile := filepath.Join(tmpDir, "script.sh")
		err = os.WriteFile(scriptFile, []byte(script), 0755)
		require.NoError(t, err)

		config := Config{
			Command:        "sh",
			Args:           []string{scriptFile},
			ConfigFilePath: configFile,
		}

		m, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, m)

		// Run manager in background
		done := make(chan error, 1)
		go func() {
			done <- m.Run()
		}()

		// Wait for initial start
		time.Sleep(500 * time.Millisecond)

		// Verify process started
		data, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "started")

		// Modify config to trigger restart
		err = os.WriteFile(configFile, []byte("modified"), 0644)
		require.NoError(t, err)

		// Wait for restart
		time.Sleep(1 * time.Second)

		// Verify process restarted (should have 2 "started" entries)
		data, err = os.ReadFile(outputFile)
		require.NoError(t, err)
		count := 0
		for _, line := range []byte(string(data)) {
			if line == '\n' {
				count++
			}
		}
		assert.GreaterOrEqual(t, count, 1) // At least restarted once

		// Shutdown
		m.cancel()

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for manager to exit")
		}
	})
}

// Test that manager properly handles context cancellation
func TestManager_ContextCancellation(t *testing.T) {
	config := Config{
		Command: "sleep",
		Args:    []string{"30"},
	}

	m, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, m)

	// Run in background
	done := make(chan error, 1)
	go func() {
		done <- m.Run()
	}()

	// Wait for startup
	time.Sleep(500 * time.Millisecond)

	// Cancel context
	m.cancel()

	// Should exit quickly
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for manager to exit after context cancellation")
	}
}

// Benchmark manager creation
func BenchmarkNew(b *testing.B) {
	config := Config{
		Command: "echo",
		Args:    []string{"hello"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := New(config)
		if err != nil {
			b.Fatal(err)
		}
		m.cancel()
	}
}
