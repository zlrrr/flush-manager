# ConfigMap 问题修复总结

## 问题描述

生产环境中，修改 Kubernetes ConfigMap `/usr/local/bin/conf/exporter.conf` 后，flush-manager 未能检测到文件变化，导致 redis-exporter 进程没有重启。

## 根本原因

Kubernetes ConfigMap 使用**符号链接（symlink）**机制来更新文件：

```
/usr/local/bin/conf/
├── exporter.conf -> ..data/exporter.conf  (符号链接)
├── ..data -> ..2023_11_02_12_00_00.123   (符号链接到目录)
└── ..2023_11_02_12_00_00.123/
    └── exporter.conf                      (实际文件)
```

当 ConfigMap 更新时：
1. Kubelet 创建新的时间戳目录
2. **原子性地更新 `..data` 符号链接**
3. 文件路径不变，但 **inode 会改变**

原有的文件监听实现只监听文件本身的写事件，无法检测到符号链接目标的变化。

## 解决方案

### 1. 符号链接检测与处理

```go
// 启动时检测符号链接
fileInfo, err := os.Lstat(filePath)
isSymlink := fileInfo.Mode()&os.ModeSymlink != 0

if isSymlink {
    realPath, _ := filepath.EvalSymlinks(filePath)
    logger.Info("Config file %s is a symlink pointing to %s", filePath, realPath)
}
```

**日志输出示例**：
```
[flush-manager] INFO: Config file /usr/local/bin/conf/exporter.conf is a symlink pointing to /usr/local/bin/conf/..data_v1/exporter.conf
```

### 2. 多级目录监听

监听三个层级的目录来捕获 ConfigMap 更新：

```go
dir := filepath.Dir(filePath)              // /usr/local/bin/conf
watcher.Add(dir)

if isSymlink {
    configDir := filepath.Dir(dir)         // /usr/local/bin (包含 ..data)
    watcher.Add(configDir)
}
```

**日志输出示例**：
```
[flush-manager] INFO: Watching directory: /usr/local/bin/conf
[flush-manager] INFO: Watching config directory for ConfigMap updates: /usr/local/bin
```

### 3. Inode 跟踪

通过跟踪文件 inode 来检测符号链接目标变化：

```go
var lastInode uint64

// 检查文件变化
if sysStat, ok := stat.Sys().(*syscall.Stat_t); ok {
    inode = sysStat.Ino
}

if modTime.After(lastModTime) || (inode != 0 && inode != lastInode) {
    logger.Info("File change detected: old_inode=%d, new_inode=%d", lastInode, inode)
    return true
}
```

**日志输出示例**：
```
[flush-manager] DEBUG: Initial file state: mtime=2025-11-02 13:20:57, inode=8280
[flush-manager] INFO: File change detected: old_inode=8280, new_inode=8315
```

### 4. 轮询备份机制

添加 5 秒轮询作为备份，确保即使 fsnotify 遗漏事件也能检测变化：

```go
func (fw *fileWatcher) poll(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    for {
        select {
        case <-ticker.C:
            if fw.checkFileChanged() {
                logger.Info("File change detected via polling")
                fw.changeChan <- struct{}{}
            }
        }
    }
}
```

**日志输出示例**：
```
[flush-manager] DEBUG: Started polling file changes every 5s
[flush-manager] INFO: File change detected via polling
```

### 5. ConfigMap 事件过滤

特别处理 ConfigMap 相关的文件系统事件：

```go
eventBase := filepath.Base(event.Name)
if eventBase == "..data" || eventBase == "..data_tmp" {
    shouldCheck = true
    logger.Debug("Event on ConfigMap metadata: %s", event.Name)
}
```

**日志输出示例**：
```
[flush-manager] DEBUG: Fsnotify event: REMOVE /usr/local/bin/conf/..data
[flush-manager] DEBUG: Event on ConfigMap metadata: /usr/local/bin/conf/..data
```

## 完整的日志流程

当 ConfigMap 更新时，您会看到以下日志序列：

```bash
# 1. 启动检测
[flush-manager] INFO: Config file /usr/local/bin/conf/exporter.conf is a symlink pointing to /usr/local/bin/conf/..data/exporter.conf
[flush-manager] INFO: Watching directory: /usr/local/bin/conf
[flush-manager] INFO: Watching config directory for ConfigMap updates: /usr/local/bin
[flush-manager] DEBUG: Initial file state: mtime=2025-11-02 12:00:00, inode=12345
[flush-manager] DEBUG: Started fsnotify event loop
[flush-manager] DEBUG: Started polling file changes every 5s

# 2. ConfigMap 更新检测（fsnotify 方式）
[flush-manager] DEBUG: Fsnotify event: REMOVE /usr/local/bin/conf/..data
[flush-manager] DEBUG: Event on ConfigMap metadata: /usr/local/bin/conf/..data
[flush-manager] DEBUG: Detected relevant file event: REMOVE
[flush-manager] INFO: File change detected: old_mtime=2025-11-02 12:00:00, new_mtime=2025-11-02 12:05:30, old_inode=12345, new_inode=67890
[flush-manager] INFO: File change confirmed after debounce period
[flush-manager] DEBUG: Change notification sent via fsnotify

# 3. 进程重启
[flush-manager] INFO: Config file change detected, restarting child process...
[flush-manager] INFO: Restarting child process...
[flush-manager] INFO: Stopping child process (PID: 123) with timeout: 10s
[flush-manager] DEBUG: Sent SIGTERM to process (PID: 123), waiting for graceful shutdown...
[flush-manager] INFO: Child process (PID: 123) stopped gracefully
[flush-manager] INFO: Restarting child process after stop
[flush-manager] INFO: Starting child process: /usr/local/bin/redis-exporter []
[flush-manager] INFO: Child process started with PID: 456
[flush-manager] INFO: Child process restarted successfully after config change
```

或通过轮询检测：

```bash
# 轮询检测（如果 fsnotify 遗漏）
[flush-manager] INFO: File change detected via polling
[flush-manager] INFO: File change detected: old_inode=12345, new_inode=67890
[flush-manager] DEBUG: Change notification sent via polling
[flush-manager] INFO: Config file change detected, restarting child process...
```

## 诊断命令

### 1. 检查文件监听是否启动

```bash
kubectl logs <pod-name> | grep "Starting file watcher"
```

预期输出：
```
[flush-manager] INFO: Starting file watcher for /usr/local/bin/conf/exporter.conf
```

### 2. 检查符号链接检测

```bash
kubectl logs <pod-name> | grep "symlink"
```

预期输出：
```
[flush-manager] INFO: Config file /usr/local/bin/conf/exporter.conf is a symlink pointing to ...
```

### 3. 检查变化检测

```bash
kubectl logs <pod-name> | grep "File change detected"
```

预期输出：
```
[flush-manager] INFO: File change detected: old_inode=12345, new_inode=67890
```

### 4. 查看所有 fsnotify 事件（调试）

```bash
kubectl logs <pod-name> | grep "Fsnotify event"
```

### 5. 检查轮询是否工作

```bash
kubectl logs <pod-name> | grep "polling"
```

预期输出：
```
[flush-manager] DEBUG: Started polling file changes every 5s
```

### 6. 验证进程重启

```bash
kubectl logs <pod-name> | grep -E "restarting|Restarting"
```

预期输出：
```
[flush-manager] INFO: Config file change detected, restarting child process...
[flush-manager] INFO: Restarting child process...
[flush-manager] INFO: Child process restarted successfully after config change
```

## 性能影响

- **内存开销**：轮询机制几乎无额外内存开销
- **CPU 开销**：每 5 秒一次 `stat()` 系统调用，可忽略不计
- **响应时间**：
  - Fsnotify 检测：< 1 秒（通常即时）
  - 轮询检测：最多 5 秒
  - 防抖延迟：500 毫秒
  - **总最大延迟**：约 6 秒

## 兼容性

- ✅ Go 1.13 - 1.23
- ✅ Kubernetes ConfigMap 挂载
- ✅ 常规文件（非符号链接）
- ✅ NFS/网络文件系统（通过轮询）
- ✅ Linux inotify 支持

## 测试验证

1. **创建测试 Pod**：
   ```yaml
   apiVersion: v1
   kind: Pod
   metadata:
     name: test-manager
   spec:
     containers:
     - name: manager
       image: your-registry/flush-manager:latest
       volumeMounts:
       - name: config
         mountPath: /usr/local/bin/conf
     volumes:
     - name: config
       configMap:
         name: test-config
   ```

2. **查看启动日志**：
   ```bash
   kubectl logs test-manager
   ```

   确认看到符号链接检测日志。

3. **更新 ConfigMap**：
   ```bash
   kubectl edit configmap test-config
   ```

4. **观察日志**：
   ```bash
   kubectl logs -f test-manager
   ```

   应该在 5 秒内看到文件变化检测和进程重启日志。

## 故障排除

详见 [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 下一步优化建议

如果需要更快的响应时间，可以调整轮询间隔：

```go
// internal/watcher/watcher.go
pollInterval: 2 * time.Second,  // 从 5 秒改为 2 秒
```

权衡：更短的间隔 = 更快检测，但更高的 CPU 使用率（仍然很低）。

## 相关文件

- `internal/watcher/watcher.go` - 文件监听实现
- `internal/logger/logger.go` - 日志工具
- `TROUBLESHOOTING.md` - 详细故障排查指南
- `README.md` - 更新的文档，包含 ConfigMap 支持说明
