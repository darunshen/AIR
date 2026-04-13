# AIR 接口设计文档

## 1. 设计原则

- 统一使用 HTTP + JSON
- 接口语义与 CLI 一致
- 返回标准化执行结果
- 先支持同步执行，后续扩展流式输出

## 2. 通用约定

### 2.1 Base URL

```text
/api/v1
```

### 2.2 通用响应格式

成功：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

失败：

```json
{
  "code": 1001,
  "message": "session not found"
}
```

## 3. Run 接口

### 3.1 执行一次性任务

`POST /api/v1/run`

请求：

```json
{
  "command": "python3 /workspace/main.py",
  "timeout": 10,
  "network": "none",
  "cpu": 1,
  "memory_mb": 512
}
```

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "stdout": "hello",
    "stderr": "",
    "exit_code": 0,
    "duration_ms": 1280
  }
}
```

## 4. Session 接口

### 4.1 创建 Session

`POST /api/v1/sessions`

请求：

```json
{
  "network": "none",
  "cpu": 1,
  "memory_mb": 512,
  "ttl_minutes": 30
}
```

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "session_id": "sess_123",
    "status": "running"
  }
}
```

### 4.2 查询 Session

`GET /api/v1/sessions/{session_id}`

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "session_id": "sess_123",
    "vm_id": "vm_001",
    "status": "idle",
    "created_at": "2026-04-13T16:00:00Z",
    "last_used_at": "2026-04-13T16:05:00Z"
  }
}
```

### 4.3 在 Session 中执行命令

`POST /api/v1/sessions/{session_id}/exec`

请求：

```json
{
  "command": "ls -al /workspace",
  "timeout": 5
}
```

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "stdout": "total 4",
    "stderr": "",
    "exit_code": 0,
    "duration_ms": 120
  }
}
```

### 4.4 删除 Session

`DELETE /api/v1/sessions/{session_id}`

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "session_id": "sess_123",
    "status": "deleted"
  }
}
```

## 5. 健康检查接口

### 5.1 服务健康检查

`GET /api/v1/health`

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "healthy"
  }
}
```

## 6. 错误码建议

- `1001`：session 不存在
- `1002`：vm 启动失败
- `1003`：exec 超时
- `1004`：guest agent 不可用
- `1005`：资源不足
- `1006`：非法参数

## 7. 后续扩展

- WebSocket 流式输出
- 批量任务接口
- snapshot/restore 接口
- 文件上传下载接口
