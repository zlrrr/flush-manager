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
3. **Automatic Restart**: When the configuration file changes, the manager gracefully restarts the child process
4. **Exit Handling**:
   - If the child process exits abnormally, the manager also exits
   - If the manager restarts the child process, it continues running
5. **Signal Handling**: The manager catches SIGTERM/SIGINT and performs graceful shutdown

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

- Go 1.19 or later

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
└── README.md
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
