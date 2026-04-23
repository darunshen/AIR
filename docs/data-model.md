# AIR 数据结构设计

## 1. 设计目标

数据模型需要覆盖任务执行、会话管理、VM 生命周期、资源限制与结果回传。MVP 先支持本地 JSON 存储，后续可平滑迁移到 SQLite 或 Postgres。

## 2. 核心对象

## 2.1 Session

表示一个长期存在的隔离执行环境。

```go
type Session struct {
    ID         string    `json:"id"`
    VMID       string    `json:"vm_id"`
    Status     string    `json:"status"`
    Network    string    `json:"network"`
    CPU        int       `json:"cpu"`
    MemoryMB   int       `json:"memory_mb"`
    TTLMinutes int       `json:"ttl_minutes"`
    CreatedAt  time.Time `json:"created_at"`
    LastUsedAt time.Time `json:"last_used_at"`
}
```

状态建议：

```text
created / running / idle / stopped / deleted / error
```

## 2.2 VM

表示底层虚拟机实例。

```go
type VM struct {
    ID                string    `json:"id"`
    SessionID         string    `json:"session_id"`
    Status            string    `json:"status"`
    RootfsPath        string    `json:"rootfs_path"`
    WorkspacePath     string    `json:"workspace_path,omitempty"`
    WorkspaceImagePath string   `json:"workspace_image_path,omitempty"`
    WorkspaceUpperPath string   `json:"workspace_upper_path,omitempty"`
    PID               int       `json:"pid"`
    CreatedAt         time.Time `json:"created_at"`
}
```

## 2.3 ExecRequest

表示一次执行请求。

```go
type ExecRequest struct {
    SessionID string `json:"session_id"`
    Command   string `json:"command"`
    Timeout   int    `json:"timeout"`
}
```

## 2.4 ExecResult

表示一次执行结果。

```go
type ExecResult struct {
    Stdout     string `json:"stdout"`
    Stderr     string `json:"stderr"`
    ExitCode   int    `json:"exit_code"`
    DurationMS int64  `json:"duration_ms"`
}
```

## 2.5 RunRequest

表示一次性任务请求。

```go
type RunRequest struct {
    Command  string `json:"command"`
    Timeout  int    `json:"timeout"`
    Network  string `json:"network"`
    CPU      int    `json:"cpu"`
    MemoryMB int    `json:"memory_mb"`
}
```

## 3. 本地存储结构

### 3.1 sessions.json

MVP 阶段可使用以下结构：

```json
{
  "sessions": [
    {
      "id": "sess_123",
      "vm_id": "vm_001",
      "status": "running",
      "network": "none",
      "cpu": 1,
      "memory_mb": 512,
      "ttl_minutes": 30,
      "created_at": "2026-04-13T16:00:00Z",
      "last_used_at": "2026-04-13T16:05:00Z"
    }
  ]
}
```

## 4. Guest 通信数据格式

### 4.1 命令消息

```json
{
  "type": "exec",
  "request_id": "req_001",
  "command": "ls -al",
  "timeout": 5
}
```

### 4.2 执行结果消息

```json
{
  "type": "result",
  "request_id": "req_001",
  "stdout": "file.txt",
  "stderr": "",
  "exit_code": 0
}
```

## 5. 生命周期状态机

### 5.1 Session 状态机

```text
CREATED -> RUNNING -> IDLE -> STOPPED -> DELETED
```

异常补充：

```text
RUNNING -> ERROR
IDLE -> ERROR
```

### 5.2 VM 状态机

```text
CREATED -> BOOTING -> RUNNING -> STOPPED -> DESTROYED
```

## 6. 目录结构建议

```text
data/
  sessions.json
runtime/
  sessions/
    firecracker/
      sess_123/
        rootfs.ext4
        workspace.ext4
        workspace-upper.ext4
        console.log
        events.jsonl
        config/
    local/
      sess_456/
        workspace/
        task/
        events.jsonl
```

## 7. 后续演进

- 增加 Task 表，记录历史执行
- 增加 Snapshot 元数据
- 增加租户与权限模型
- 增加指标对象，如启动时延、exec 时延、回收结果
