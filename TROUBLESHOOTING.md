# Troubleshooting Guide

This guide helps you diagnose and fix common issues with the flush-manager.

## Understanding the Logs

All log messages are prefixed with `[flush-manager]` and include timestamps. Log levels:

- **INFO**: Important operational messages
- **ERROR**: Error conditions
- **DEBUG**: Detailed diagnostic information

### Log Categories

1. **Startup Logs**
   ```
   [flush-manager] INFO: === Flush Manager v1.0.0 starting ===
   [flush-manager] INFO: PID: 1
   [flush-manager] INFO: Configuration: command=/usr/local/bin/redis-exporter, config_file=/usr/local/bin/conf/exporter.conf
   [flush-manager] INFO: Initializing manager with command: /usr/local/bin/redis-exporter
   ```

2. **File Watcher Logs**
   ```
   [flush-manager] INFO: Config file /path/to/config is a symlink pointing to /real/path
   [flush-manager] INFO: Watching directory: /usr/local/bin/conf
   [flush-manager] INFO: Starting file watcher for /path/to/config
   [flush-manager] DEBUG: Started fsnotify event loop
   [flush-manager] DEBUG: Started polling file changes every 5s
   ```

3. **File Change Detection Logs**
   ```
   [flush-manager] DEBUG: Fsnotify event: REMOVE /usr/local/bin/conf/..data
   [flush-manager] DEBUG: Event on ConfigMap metadata: /usr/local/bin/conf/..data
   [flush-manager] INFO: File change detected: old_mtime=..., new_mtime=..., old_inode=..., new_inode=...
   [flush-manager] INFO: File change confirmed after debounce period
   [flush-manager] INFO: Config file change detected, restarting child process...
   ```

4. **Process Management Logs**
   ```
   [flush-manager] INFO: Starting child process: /usr/local/bin/redis-exporter []
   [flush-manager] INFO: Child process started with PID: 123
   [flush-manager] INFO: Restarting child process...
   [flush-manager] INFO: Stopping child process (PID: 123) with timeout: 10s
   [flush-manager] DEBUG: Sent SIGTERM to process (PID: 123), waiting for graceful shutdown...
   [flush-manager] INFO: Child process (PID: 123) stopped gracefully
   ```

## Common Issues

### Issue 1: ConfigMap Updates Not Detected

**Symptoms:**
- You update the ConfigMap in Kubernetes
- The manager doesn't restart the child process
- No "Config file change detected" messages in logs

**Diagnosis Steps:**

1. **Check if file watcher started correctly:**
   ```bash
   # Look for these logs in your pod:
   kubectl logs <pod-name> | grep "Starting file watcher"
   ```

   Expected output:
   ```
   [flush-manager] INFO: Config file /usr/local/bin/conf/exporter.conf is a symlink pointing to /usr/local/bin/conf/..data/exporter.conf
   [flush-manager] INFO: Watching directory: /usr/local/bin/conf
   [flush-manager] INFO: Starting file watcher for /usr/local/bin/conf/exporter.conf
   ```

2. **Check if the file is a symlink (ConfigMap behavior):**
   ```bash
   kubectl exec <pod-name> -- ls -la /usr/local/bin/conf/
   ```

   You should see something like:
   ```
   exporter.conf -> ..data/exporter.conf
   ..data -> ..2023_11_02_12_00_00.123456789
   ```

3. **Check fsnotify events:**
   ```bash
   kubectl logs <pod-name> | grep "Fsnotify event"
   ```

   When ConfigMap updates, you should see events like:
   ```
   [flush-manager] DEBUG: Fsnotify event: REMOVE /usr/local/bin/conf/..data
   [flush-manager] DEBUG: Event on ConfigMap metadata: /usr/local/bin/conf/..data
   ```

4. **Check polling fallback:**

   The manager includes a polling mechanism (every 5 seconds) as a fallback:
   ```bash
   kubectl logs <pod-name> | grep "polling"
   ```

   You should see:
   ```
   [flush-manager] DEBUG: Started polling file changes every 5s
   [flush-manager] INFO: File change detected via polling
   ```

**Solutions:**

- **If file watcher is not starting:** Check that the config file path is correct and the file exists
- **If no fsnotify events:** This is normal for ConfigMap updates; the polling mechanism should catch changes
- **If polling is not working:** Check if the file's modification time or inode changes when ConfigMap updates

### Issue 2: Process Crashes But Manager Doesn't Exit

**Symptoms:**
- Child process crashes
- Manager keeps running
- You expect the manager to exit

**Diagnosis:**

Check the exit reason in logs:
```bash
kubectl logs <pod-name> | grep "Process exit"
```

Expected log when process exits abnormally:
```
[flush-manager] INFO: Child process exited abnormally: exit status 1
[flush-manager] ERROR: Child process exited with error: exit status 1
[flush-manager] INFO: Shutting down manager...
```

If you see `Process exited due to restart request`, the manager restarted it intentionally.

### Issue 3: High CPU Usage

**Symptoms:**
- Manager using excessive CPU
- Many DEBUG log messages

**Diagnosis:**

1. Check for excessive fsnotify events:
   ```bash
   kubectl logs <pod-name> | grep "Fsnotify event" | wc -l
   ```

2. Check polling frequency:
   ```bash
   kubectl logs <pod-name> | grep "Started polling"
   ```

**Solution:**

The polling interval is set to 5 seconds by default, which is reasonable. If you see excessive fsnotify events, there might be another process modifying files in the watched directory.

### Issue 4: Manager Takes Too Long to Detect Changes

**Symptoms:**
- ConfigMap update applied
- Manager takes more than 10 seconds to restart process

**Expected Behavior:**

- **Fsnotify detection:** Near-instant (< 1 second)
- **Polling detection:** Up to 5 seconds
- **Debounce period:** 500ms after last event

**Total maximum delay:** ~6 seconds

**Diagnosis:**

Check when the change was detected:
```bash
kubectl logs <pod-name> | grep -E "File change detected|Config file change detected"
```

## Kubernetes-Specific Considerations

### ConfigMap Mount Behavior

Kubernetes mounts ConfigMaps using a complex symlink structure:

```
/usr/local/bin/conf/
├── exporter.conf -> ..data/exporter.conf
├── ..data -> ..2023_11_02_12_00_00.123456789/
├── ..2023_11_02_12_00_00.123456789/
│   └── exporter.conf (actual file)
└── ..2023_11_02_11_00_00.987654321/ (old version)
```

When ConfigMap updates:
1. Kubelet creates a new timestamped directory
2. Kubelet updates the `..data` symlink atomically
3. File inode changes even though the path stays the same

The manager handles this by:
- Detecting symlinks on startup
- Watching for `..data` directory changes
- Tracking file inode changes
- Using polling as a fallback

### Testing in Kubernetes

1. **Update ConfigMap:**
   ```bash
   kubectl edit configmap <configmap-name>
   ```

2. **Watch manager logs:**
   ```bash
   kubectl logs -f <pod-name>
   ```

3. **Expected log sequence:**
   ```
   [flush-manager] DEBUG: Fsnotify event: REMOVE /usr/local/bin/conf/..data
   [flush-manager] DEBUG: Event on ConfigMap metadata: ...
   [flush-manager] INFO: File change detected: old_inode=..., new_inode=...
   [flush-manager] INFO: File change confirmed after debounce period
   [flush-manager] INFO: Config file change detected, restarting child process...
   [flush-manager] INFO: Restarting child process...
   [flush-manager] INFO: Stopping child process (PID: X) with timeout: 10s
   [flush-manager] INFO: Child process (PID: X) stopped gracefully
   [flush-manager] INFO: Starting child process: ...
   [flush-manager] INFO: Child process started with PID: Y
   [flush-manager] INFO: Child process restarted successfully after config change
   ```

## Debug Mode

To enable more verbose logging, the manager outputs DEBUG level logs by default. These logs help diagnose:

- File system events (all fsnotify events)
- Polling checks
- Process lifecycle details
- Signal handling

## Getting Help

When reporting issues, include:

1. **Full log output:** `kubectl logs <pod-name> > manager.log`
2. **File structure:** `kubectl exec <pod-name> -- ls -laR /usr/local/bin/conf/`
3. **ConfigMap content:** `kubectl get configmap <name> -o yaml`
4. **Pod description:** `kubectl describe pod <pod-name>`
5. **Manager version:** Check the startup log for version number

## Performance Tuning

### Polling Interval

If you need faster ConfigMap update detection, you can modify the `pollInterval` in `internal/watcher/watcher.go`:

```go
pollInterval: 2 * time.Second, // Default is 5 seconds
```

Trade-off: Lower interval = faster detection, but higher CPU usage.

### Debounce Period

To reduce restart frequency during rapid ConfigMap changes:

```go
debounce: 1 * time.Second, // Default is 500ms
```

Trade-off: Longer debounce = fewer restarts, but slower response.

## Version Compatibility

- **Go 1.13+**: Minimum version (production uses Go 1.13)
- **Go 1.23**: Tested and compatible
- **Kubernetes 1.19+**: ConfigMap behavior consistent
- **Linux kernel 2.6.13+**: Required for inotify (fsnotify)

## Known Limitations

1. **NFS/Network filesystems:** Fsnotify may not work reliably. Polling fallback helps but adds latency.
2. **Many files in watched directory:** Can cause high fsnotify event volume.
3. **File systems without inode support:** Inode-based change detection won't work, but mtime-based detection still functions.
