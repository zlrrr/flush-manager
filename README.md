# Flush Manager

A lightweight, efficient init process manager (PID 1) for container environments, written in Go.

## Features

- **PID 1 Process Manager**: Designed to run as the init process in containers
- **Process Supervision**: Automatically monitors and manages child processes
- **Configuration Hot-Reload**: Watches for configuration file changes and gracefully restarts the managed process
- **Graceful Shutdown**: Handles SIGTERM/SIGINT signals and performs clean shutdown
- **Lightweight**: Minimal resource usage and simple design
- **Well-Tested**: Comprehensive test coverage with unit tests

## Installation

### From Source

```bash
go build -o manager ./cmd/manager
```

### Using Makefile

```bash
make build
```

## Usage

### Basic Usage

```bash
# Run with default settings (redis-exporter)
./manager

# Run with custom command
./manager -command /path/to/your/app

# Run with custom command and arguments
./manager -command /usr/bin/myapp arg1 arg2

# Specify config file to watch
./manager -command /usr/bin/myapp -config /etc/myapp/config.conf
```

### Command Line Options

- `-command`: Command to execute (default: `/usr/local/bin/redis-exporter`)
- `-config`: Configuration file to watch for changes (default: `/usr/local/bin/conf/exporter.conf`)
- `-version`: Print version information

### Docker Example

```dockerfile
FROM alpine:latest

COPY manager /usr/local/bin/manager
COPY your-app /usr/local/bin/your-app
COPY config.conf /usr/local/bin/conf/config.conf

# Use manager as PID 1
ENTRYPOINT ["/usr/local/bin/manager"]
CMD ["-command", "/usr/local/bin/your-app"]
```

## How It Works

1. **Process Management**: The manager starts the specified child process and monitors its lifecycle
2. **Configuration Watching**: If a configuration file path is provided and the file exists, the manager watches for file modifications
   - Uses fsnotify for real-time file system events
   - Includes polling fallback (every 5 seconds) for reliable detection
   - Handles Kubernetes ConfigMap updates via symlink/inode tracking
3. **Automatic Restart**: When the configuration file changes, the manager gracefully restarts the child process
4. **Exit Handling**:
   - If the child process exits abnormally, the manager also exits
   - If the manager restarts the child process, it continues running
5. **Signal Handling**: The manager catches SIGTERM/SIGINT and performs graceful shutdown

## Logging

All log messages are prefixed with `[flush-manager]` to make them easy to identify in combined logs. The manager logs at different levels:

- **INFO**: Important operational messages (startup, shutdown, config changes, process lifecycle)
- **ERROR**: Error conditions
- **DEBUG**: Detailed diagnostic information (file system events, internal state)

Example log output:
```
[flush-manager] INFO: === Flush Manager v1.0.0 starting ===
[flush-manager] INFO: PID: 1
[flush-manager] INFO: Configuration: command=/usr/local/bin/redis-exporter, config_file=/usr/local/bin/conf/exporter.conf
[flush-manager] INFO: Config file /usr/local/bin/conf/exporter.conf is a symlink pointing to /usr/local/bin/conf/..data/exporter.conf
[flush-manager] INFO: Watching directory: /usr/local/bin/conf
[flush-manager] INFO: Starting child process: /usr/local/bin/redis-exporter []
[flush-manager] INFO: Child process started with PID: 123
[flush-manager] INFO: File change detected: old_inode=456, new_inode=789
[flush-manager] INFO: Config file change detected, restarting child process...
```

## Kubernetes ConfigMap Support

The manager is specifically designed to work with Kubernetes ConfigMap mounts:

### How ConfigMaps Are Mounted

Kubernetes mounts ConfigMaps using symlinks:
```
/usr/local/bin/conf/
├── exporter.conf -> ..data/exporter.conf (symlink to file)
├── ..data -> ..2023_11_02_12_00_00.123456789 (symlink to directory)
└── ..2023_11_02_12_00_00.123456789/
    └── exporter.conf (actual file)
```

When you update a ConfigMap, Kubernetes:
1. Creates a new timestamped directory with updated files
2. Atomically updates the `..data` symlink
3. Eventually removes old directories

### How the Manager Handles This

1. **Symlink Detection**: On startup, detects if the config file is a symlink
2. **Multi-Level Watching**: Watches both the file and parent directories
3. **Inode Tracking**: Detects when symlink target changes (inode changes)
4. **Polling Fallback**: Checks every 5 seconds to ensure changes aren't missed
5. **Debouncing**: Waits 500ms after last change to avoid multiple restarts

### Example in Kubernetes

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: redis-exporter
spec:
  containers:
  - name: exporter
    image: your-registry/redis-exporter:latest
    command: ["/usr/local/bin/manager"]
    volumeMounts:
    - name: config
      mountPath: /usr/local/bin/conf
  volumes:
  - name: config
    configMap:
      name: redis-exporter-config
```

When you run `kubectl edit configmap redis-exporter-config`, the manager will:
1. Detect the ConfigMap update within 5 seconds (or instantly via fsnotify)
2. Log the change with old and new inode numbers
3. Gracefully restart the redis-exporter process
4. Continue running normally

## Troubleshooting

If the manager is not detecting config file changes, check:

1. **Verify file watcher started:**
   ```bash
   kubectl logs <pod-name> | grep "Starting file watcher"
   ```

2. **Check for change detection:**
   ```bash
   kubectl logs <pod-name> | grep "File change detected"
   ```

3. **Monitor fsnotify events (debug):**
   ```bash
   kubectl logs <pod-name> | grep "Fsnotify event"
   ```

4. **Verify polling is active:**
   ```bash
   kubectl logs <pod-name> | grep "polling"
   ```

For detailed troubleshooting, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Architecture

The project is structured into three main components:

### Process Manager (`internal/process`)
- Manages child process lifecycle (start, stop, restart)
- Tracks process exit reasons (abnormal vs. restart)
- Handles graceful termination with timeout

### File Watcher (`internal/watcher`)
- Monitors configuration file changes using fsnotify
- Implements debouncing to avoid multiple rapid restarts
- Handles file recreation and modification events

### Core Manager (`internal/manager`)
- Coordinates process management and file watching
- Handles signal processing (SIGTERM, SIGINT)
- Implements the main event loop

## Development

### Prerequisites

- Go 1.13 or later (tested with Go 1.13 - 1.23)

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests with verbose output
go test -v ./...
```

### Project Structure

```
flush-manager/
├── cmd/
│   └── manager/          # Main application entry point
│       └── main.go
├── internal/
│   ├── logger/           # Logging utilities
│   │   └── logger.go
│   ├── manager/          # Core manager logic
│   │   ├── manager.go
│   │   └── manager_test.go
│   ├── process/          # Process management
│   │   ├── process.go
│   │   └── process_test.go
│   └── watcher/          # File watching
│       ├── watcher.go
│       └── watcher_test.go
├── go.mod
├── go.sum
├── Makefile
├── LICENSE
├── README.md
└── TROUBLESHOOTING.md
```

## Testing

The project includes comprehensive tests for all components:

- **Process Manager Tests**: Process lifecycle, restart behavior, signal handling
- **File Watcher Tests**: File change detection, debouncing, edge cases
- **Manager Tests**: Integration tests, shutdown behavior, configuration changes

All external interactions are properly mocked to ensure reliable and fast tests.

## Design Decisions

### Lightweight Design
- Minimal dependencies (only fsnotify for file watching)
- Simple, focused functionality
- Efficient resource usage

### Graceful Handling
- 10-second timeout for graceful process termination
- Automatic fallback to SIGKILL if needed
- Proper cleanup of all resources

### Null Object Pattern
- File watcher returns a no-op implementation when file doesn't exist
- Avoids nil pointer checks throughout the codebase
- Cleaner, more maintainable code

### Process Group Management
- Child process runs in its own process group
- Prevents signal propagation issues
- Better isolation

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please ensure:
1. All tests pass: `make test`
2. Code is properly formatted: `go fmt ./...`
3. New features include tests

## Version

Current version: 1.0.0
