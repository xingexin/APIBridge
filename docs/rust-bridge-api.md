# Rust 桥接内部接口

`GPTBridge` 将 Rust 服务视为内部上游。Rust 负责维护逆向链路细节，例如会话、设备指纹、PoW、文件上传流程以及上游 SSE 处理。

Go 只调用稳定的内部能力接口。

## 基础地址

- 本地开发：`http://127.0.0.1:8081`
- Go 侧通过 `RUST_BRIDGE_BASE_URL` 配置

## Go 透传的请求头

- `Authorization`
- `X-Request-Id`
- `X-Account-Id`
- `X-Model-Override`
- `Accept`

后续 Rust 可以扩展更多请求头，但 Go 侧应保持克制，只透传显式支持的字段。

## 接口列表

### `POST /internal/chat/completions`

用途：
- 对逆向出的 ChatGPT 上游执行聊天补全请求。

请求体：
- 直接接收来自 `/v1/chat/completions` 的 OpenAI 兼容 JSON。

响应：
- 非流式：JSON 响应体
- 流式：`text/event-stream`

### `POST /internal/responses`

用途：
- 对逆向上游执行 responses 请求。

请求体：
- 直接接收来自 `/v1/responses` 的 OpenAI 兼容 JSON。

响应：
- 根据 `stream` 返回 JSON 或 SSE。

### `POST /internal/images/generations`

用途：
- 调用逆向上游执行图片生成。

请求体：
- 直接接收来自 `/v1/images/generations` 的 OpenAI 兼容 JSON。

响应：
- JSON 格式的图片生成结果。

### `POST /internal/images/edits`

用途：
- 调用逆向上游执行图片编辑。

请求体：
- 直接接收来自 `/v1/images/edits` 的 OpenAI 兼容 JSON。

响应：
- JSON 格式的图片编辑结果。

### `POST /internal/files`

用途：
- 将文件上传到 Rust，由 Rust 继续执行上游上传流程。

表单字段：
- `file`：二进制文件
- `purpose`：上传用途，例如 `vision`

请求头：
- 可选 `X-File-Content-Type`

响应示例：

```json
{
  "id": "file_123",
  "object": "file",
  "filename": "input.png",
  "bytes": 1024
}
```

### `GET /internal/models`

用途：
- 返回 Rust 当前可提供的模型列表。

响应：
- OpenAI 兼容的原始模型列表 JSON。

### `GET /internal/health`

用途：
- 检查 Rust 桥接服务健康状态，也可附带检查某个账号。

查询参数：
- `account_id`：可选

响应示例：

```json
{
  "status": "ok",
  "account_id": "acc_1",
  "message": "healthy"
}
```

## 错误约定

Rust 返回错误时，建议尽量使用 JSON 格式：

```json
{
  "error": {
    "message": "account is rate limited",
    "type": "rate_limit",
    "code": "account_limited"
  }
}
```

Go 当前会将状态码和响应体文本包装成桥接错误返回。后续如果需要更丰富的类型化错误，也可以在不改接口路径的前提下继续演进。
