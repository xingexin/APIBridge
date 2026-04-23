# Rust 桥接 RPC 契约

Go 和 Rust 之间使用明文 gRPC h2c 通信，默认地址：

```text
127.0.0.1:50051
```

Go 侧通过环境变量配置：

```bash
RUST_BRIDGE_GRPC_ADDR=127.0.0.1:50051
```

## 服务方法

服务名：

```text
gptbridge.bridge.v1.BridgeService
```

方法：

```text
rpc StreamProxy(google.protobuf.Struct) returns (stream google.protobuf.Struct)
```

Go 侧目前使用通用 `Struct` 承载请求和响应，避免早期频繁生成 pb 代码。等 Rust 侧协议稳定后，可以再替换成强类型 proto。

## 请求结构

Go 发给 Rust 的请求字段：

```json
{
  "operation": "chat_completion",
  "method": "POST",
  "path": "/v1/chat/completions",
  "headers": {
    "Authorization": "Bearer xxx",
    "X-Trace-Id": "trace-id"
  },
  "body_base64": "..."
}
```

`operation` 当前取值：

- `chat_completion`
- `response`
- `image_generation`
- `image_edit`
- `file_upload`
- `models`
- `health`
- `proxy`

## 响应流结构

Rust 返回的第一帧必须是响应元信息：

```json
{
  "status_code": 200,
  "headers": {
    "Content-Type": "text/event-stream"
  }
}
```

后续帧返回 body 分片：

```json
{
  "body_base64": "..."
}
```

如果是 SSE，Rust 可以持续发送多个 body 分片，Go 会边读边写给客户端。
