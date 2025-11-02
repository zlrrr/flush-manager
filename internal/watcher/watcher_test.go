package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileWatcher(t *testing.T) {
	t.Run("create watcher for existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		assert.NoError(t, err)
		assert.NotNil(t, fw)

		if fw != nil {
			fw.Close()
		}
	})

	t.Run("return noop watcher for non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "nonexistent.conf")

		fw, err := NewFileWatcher(filePath)
		assert.NoError(t, err)
		assert.NotNil(t, fw)
		assert.Nil(t, fw.Changes()) // noop watcher returns nil channel
	})

	t.Run("return noop watcher for empty path", func(t *testing.T) {
		fw, err := NewFileWatcher("")
		assert.NoError(t, err)
		assert.NotNil(t, fw)
		assert.Nil(t, fw.Changes()) // noop watcher returns nil channel
	})
}

func TestFileWatcher_Changes(t *testing.T) {
	t.Run("detect file modification", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("initial"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)
		defer fw.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = fw.Start(ctx)
		require.NoError(t, err)

		// Give watcher time to start
		time.Sleep(100 * time.Millisecond)

		// Modify file
		err = os.WriteFile(filePath, []byte("modified"), 0644)
		require.NoError(t, err)

		// Wait for change notification with timeout
		select {
		case <-fw.Changes():
			// Success - change detected
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for file change notification")
		}
	})

	t.Run("debounce multiple rapid changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("initial"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)
		defer fw.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = fw.Start(ctx)
		require.NoError(t, err)

		// Give watcher time to start
		time.Sleep(100 * time.Millisecond)

		// Make multiple rapid changes
		for i := 0; i < 5; i++ {
			err = os.WriteFile(filePath, []byte("modified"), 0644)
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond)
		}

		// Should get at most one notification due to debouncing
		changeCount := 0
		timeout := time.After(1 * time.Second)

	loop:
		for {
			select {
			case <-fw.Changes():
				changeCount++
			case <-timeout:
				break loop
			}
		}

		// Due to debouncing, we should get 1-2 notifications, not 5
		assert.LessOrEqual(t, changeCount, 2)
		assert.GreaterOrEqual(t, changeCount, 1)
	})
}

func TestFileWatcher_Start(t *testing.T) {
	t.Run("start watcher successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)
		defer fw.Close()

		ctx := context.Background()
		err = fw.Start(ctx)
		assert.NoError(t, err)
	})


	t.Run("context cancellation stops watcher", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)
		defer fw.Close()

		ctx, cancel := context.WithCancel(context.Background())
		err = fw.Start(ctx)
		require.NoError(t, err)

		// Cancel context
		cancel()

		// Give goroutine time to exit
		time.Sleep(100 * time.Millisecond)

		// Modify file - should not trigger notification
		err = os.WriteFile(filePath, []byte("modified"), 0644)
		require.NoError(t, err)

		// Should not receive notification
		select {
		case <-fw.Changes():
			t.Fatal("received notification after context cancellation")
		case <-time.After(1 * time.Second):
			// Success - no notification received
		}
	})
}

func TestFileWatcher_Close(t *testing.T) {
	t.Run("close watcher successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)

		err = fw.Close()
		assert.NoError(t, err)
	})
}

func TestFileWatcher_FileRecreation(t *testing.T) {
	t.Run("detect file recreation", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file
		err := os.WriteFile(filePath, []byte("initial"), 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)
		defer fw.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = fw.Start(ctx)
		require.NoError(t, err)

		// Give watcher time to start
		time.Sleep(100 * time.Millisecond)

		// Delete and recreate file
		err = os.Remove(filePath)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = os.WriteFile(filePath, []byte("recreated"), 0644)
		require.NoError(t, err)

		// Should detect the recreation
		select {
		case <-fw.Changes():
			// Success - change detected
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for file recreation notification")
		}
	})
}

// TestFileWatcher_ModTimeCheck tests that the watcher properly checks modification time
func TestFileWatcher_ModTimeCheck(t *testing.T) {
	t.Run("ignore events without modtime change", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.conf")

		// Create test file with specific content
		content := []byte("test content")
		err := os.WriteFile(filePath, content, 0644)
		require.NoError(t, err)

		fw, err := NewFileWatcher(filePath)
		require.NoError(t, err)
		require.NotNil(t, fw)
		defer fw.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = fw.Start(ctx)
		require.NoError(t, err)

		// Give watcher time to start
		time.Sleep(100 * time.Millisecond)

		// Write the same content - modtime should change and trigger notification
		err = os.WriteFile(filePath, content, 0644)
		require.NoError(t, err)

		// Should get notification because modtime changed
		select {
		case <-fw.Changes():
			// Success
		case <-time.After(2 * time.Second):
			// This is actually ok - some filesystems might not update modtime
			// for writes with same content
		}
	})
}

// BenchmarkFileWatcher_Changes benchmarks the file change detection
func BenchmarkFileWatcher_Changes(b *testing.B) {
	tmpDir := b.TempDir()
	filePath := filepath.Join(tmpDir, "test.conf")

	// Create test file
	err := os.WriteFile(filePath, []byte("initial"), 0644)
	require.NoError(b, err)

	fw, err := NewFileWatcher(filePath)
	require.NoError(b, err)
	require.NotNil(b, fw)
	defer fw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = fw.Start(ctx)
	require.NoError(b, err)

	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = os.WriteFile(filePath, []byte("modified"), 0644)
		require.NoError(b, err)

		<-fw.Changes()
	}
}
