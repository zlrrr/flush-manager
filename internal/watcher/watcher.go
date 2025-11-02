package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/zlrrr/flush-manager/internal/logger"
)

// FileWatcher watches for file changes
type FileWatcher interface {
	Start(ctx context.Context) error
	Changes() <-chan struct{}
	Close() error
}

type fileWatcher struct {
	filePath       string
	watcher        *fsnotify.Watcher
	changeChan     chan struct{}
	debounce       time.Duration
	lastModTime    time.Time
	lastInode      uint64
	pollInterval   time.Duration
	isSymlink      bool
	realPath       string
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
		logger.Debug("No config file path specified, using no-op watcher")
		return &noopWatcher{}, nil
	}

	// Check if file exists
	fileInfo, err := os.Lstat(filePath)
	if os.IsNotExist(err) {
		logger.Info("Config file %s does not exist, using no-op watcher", filePath)
		return &noopWatcher{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// Check if file is a symlink (common in Kubernetes ConfigMap mounts)
	isSymlink := fileInfo.Mode()&os.ModeSymlink != 0
	realPath := filePath

	if isSymlink {
		realPath, err = filepath.EvalSymlinks(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve symlink %s: %w", filePath, err)
		}
		logger.Info("Config file %s is a symlink pointing to %s", filePath, realPath)
	} else {
		logger.Info("Config file %s is a regular file", filePath)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	// Watch the parent directory to catch symlink updates
	dir := filepath.Dir(filePath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}
	logger.Info("Watching directory: %s", dir)

	// Also watch the parent of the real path for ConfigMap scenarios
	if isSymlink {
		realDir := filepath.Dir(realPath)
		// In Kubernetes ConfigMaps, watch the grandparent directory which contains ..data
		configDir := filepath.Dir(dir)
		if configDir != realDir {
			if err := watcher.Add(configDir); err != nil {
				logger.Error("Failed to watch config directory %s: %v", configDir, err)
			} else {
				logger.Info("Watching config directory for ConfigMap updates: %s", configDir)
			}
		}
	}

	fw := &fileWatcher{
		filePath:     filePath,
		watcher:      watcher,
		changeChan:   make(chan struct{}, 1),
		debounce:     500 * time.Millisecond,
		pollInterval: 5 * time.Second, // Poll every 5 seconds as fallback
		isSymlink:    isSymlink,
		realPath:     realPath,
	}

	// Get initial modification time and inode
	if stat, err := os.Stat(filePath); err == nil {
		fw.lastModTime = stat.ModTime()
		if sysStat, ok := stat.Sys().(*syscall.Stat_t); ok {
			fw.lastInode = sysStat.Ino
			logger.Debug("Initial file state: mtime=%v, inode=%d", fw.lastModTime, fw.lastInode)
		}
	}

	return fw, nil
}

// Start starts watching for file changes
func (fw *fileWatcher) Start(ctx context.Context) error {
	logger.Info("Starting file watcher for %s", fw.filePath)

	// Start fsnotify watcher
	go fw.watch(ctx)

	// Start polling as a fallback (important for ConfigMaps)
	go fw.poll(ctx)

	return nil
}

// Changes returns a channel that receives notifications when the file changes
func (fw *fileWatcher) Changes() <-chan struct{} {
	return fw.changeChan
}

// Close closes the file watcher
func (fw *fileWatcher) Close() error {
	logger.Debug("Closing file watcher")
	if fw.watcher != nil {
		return fw.watcher.Close()
	}
	return nil
}

// poll checks for file changes periodically (fallback for ConfigMap scenarios)
func (fw *fileWatcher) poll(ctx context.Context) {
	ticker := time.NewTicker(fw.pollInterval)
	defer ticker.Stop()

	logger.Debug("Started polling file changes every %v", fw.pollInterval)

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Polling stopped due to context cancellation")
			return
		case <-ticker.C:
			if fw.checkFileChanged() {
				logger.Info("File change detected via polling")
				select {
				case fw.changeChan <- struct{}{}:
					logger.Debug("Change notification sent via polling")
				default:
					logger.Debug("Change notification already pending")
				}
			}
		}
	}
}

// checkFileChanged checks if the file has been modified
func (fw *fileWatcher) checkFileChanged() bool {
	stat, err := os.Stat(fw.filePath)
	if err != nil {
		logger.Error("Failed to stat file %s: %v", fw.filePath, err)
		return false
	}

	modTime := stat.ModTime()
	var inode uint64
	if sysStat, ok := stat.Sys().(*syscall.Stat_t); ok {
		inode = sysStat.Ino
	}

	// Check if either modification time or inode changed
	// Inode change indicates symlink was updated (ConfigMap scenario)
	if modTime.After(fw.lastModTime) || (inode != 0 && inode != fw.lastInode) {
		logger.Info("File change detected: old_mtime=%v, new_mtime=%v, old_inode=%d, new_inode=%d",
			fw.lastModTime, modTime, fw.lastInode, inode)
		fw.lastModTime = modTime
		fw.lastInode = inode
		return true
	}

	return false
}

// watch processes file system events
func (fw *fileWatcher) watch(ctx context.Context) {
	var debounceTimer *time.Timer
	logger.Debug("Started fsnotify event loop")

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Fsnotify watcher stopped due to context cancellation")
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				logger.Debug("Fsnotify events channel closed")
				return
			}

			logger.Debug("Fsnotify event: %s %s", event.Op, event.Name)

			// For symlinks (ConfigMap scenario), watch for changes to the symlink itself or ..data
			shouldCheck := false

			if event.Name == fw.filePath {
				// Direct file event
				shouldCheck = true
				logger.Debug("Event on target file: %s", fw.filePath)
			} else if fw.isSymlink {
				// Check for ..data or data directory changes (ConfigMap update pattern)
				eventBase := filepath.Base(event.Name)
				if eventBase == "..data" || eventBase == "..data_tmp" || eventBase == "data" {
					shouldCheck = true
					logger.Debug("Event on ConfigMap metadata: %s", event.Name)
				}
			}

			if !shouldCheck {
				continue
			}

			// Check for write, create, or remove events
			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create ||
				event.Op&fsnotify.Remove == fsnotify.Remove {

				logger.Debug("Detected relevant file event: %s", event.Op)

				// Verify file actually changed
				if !fw.checkFileChanged() {
					logger.Debug("File state unchanged, ignoring event")
					continue
				}

				// Debounce: reset timer if already running
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				debounceTimer = time.AfterFunc(fw.debounce, func() {
					logger.Info("File change confirmed after debounce period")
					select {
					case fw.changeChan <- struct{}{}:
						logger.Debug("Change notification sent via fsnotify")
					default:
						logger.Debug("Change notification already pending")
					}
				})
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				logger.Debug("Fsnotify errors channel closed")
				return
			}
			logger.Error("Fsnotify error: %v", err)
		}
	}
}
