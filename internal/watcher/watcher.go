package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches for file changes
type FileWatcher interface {
	Start(ctx context.Context) error
	Changes() <-chan struct{}
	Close() error
}

type fileWatcher struct {
	filePath    string
	watcher     *fsnotify.Watcher
	changeChan  chan struct{}
	debounce    time.Duration
	lastModTime time.Time
}

// noopWatcher is a no-op implementation of FileWatcher
type noopWatcher struct{}

func (nw *noopWatcher) Start(ctx context.Context) error {
	return nil
}

func (nw *noopWatcher) Changes() <-chan struct{} {
	return nil
}

func (nw *noopWatcher) Close() error {
	return nil
}

// NewFileWatcher creates a new file watcher
// If the file doesn't exist, it returns a no-op watcher
func NewFileWatcher(filePath string) (FileWatcher, error) {
	if filePath == "" {
		return &noopWatcher{}, nil
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File doesn't exist, return no-op watcher
		return &noopWatcher{}, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	// Watch the parent directory instead of the file directly
	// This handles cases where the file is deleted and recreated
	dir := filepath.Dir(filePath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	fw := &fileWatcher{
		filePath:   filePath,
		watcher:    watcher,
		changeChan: make(chan struct{}, 1),
		debounce:   500 * time.Millisecond, // Debounce to avoid multiple events
	}

	// Get initial modification time
	if stat, err := os.Stat(filePath); err == nil {
		fw.lastModTime = stat.ModTime()
	}

	return fw, nil
}

// Start starts watching for file changes
func (fw *fileWatcher) Start(ctx context.Context) error {
	go fw.watch(ctx)
	return nil
}

// Changes returns a channel that receives notifications when the file changes
func (fw *fileWatcher) Changes() <-chan struct{} {
	return fw.changeChan
}

// Close closes the file watcher
func (fw *fileWatcher) Close() error {
	if fw.watcher != nil {
		return fw.watcher.Close()
	}
	return nil
}

// watch processes file system events
func (fw *fileWatcher) watch(ctx context.Context) {
	var debounceTimer *time.Timer

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Only process events for our specific file
			if event.Name != fw.filePath {
				continue
			}

			// Check if file was modified or created
			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {

				// Verify modification time changed to avoid duplicate events
				stat, err := os.Stat(fw.filePath)
				if err != nil {
					continue
				}

				if !stat.ModTime().After(fw.lastModTime) {
					continue
				}

				fw.lastModTime = stat.ModTime()

				// Debounce: reset timer if already running
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				debounceTimer = time.AfterFunc(fw.debounce, func() {
					select {
					case fw.changeChan <- struct{}{}:
					default:
						// Channel already has a pending change notification
					}
				})
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Log error but continue watching
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}
}
