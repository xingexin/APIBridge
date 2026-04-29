# GPTBridge MVP 架构

当前实现把代理链路拆成四个边界：

- `biz/proxygateway`：唯一编排层，解析请求特征并串起三段短事务。
- `domain/proxy`：只按 `Route` 转发，不感知计费、帐号池、冻结、超卖。
- `domain/upstream`：管理帐号池、上游帐号、池周期、容量预占、stateful owner、冻结和 cooldown。
- `domain/billing`：管理客户 API Key、客户额度周期、客户侧预占、提交、释放和账本。

## 请求流程

1. Handler 读取请求体，调用 `proxygateway.Start`。
2. Tx1 内完成客户 API Key 解析、billing 预占、upstream resolve + reserve。
3. Tx1 结束后调用 `proxy.Forward`，不持有数据库事务等待上游响应。
4. 响应成功后，gateway 解析 usage 和资源 ID，并在 Tx2 内提交 billing/upstream、写入 resource owner。
5. 上游失败、客户端取消或流式复制失败时，Tx3 释放 billing/upstream 预占。

## 表

Billing 侧：

- `billing_accounts`
- `billing_api_keys`
- `account_quota_periods`
- `account_recharges`
- `billing_reservations`
- `billing_ledger_records`

Upstream 侧：

- `upstream_pools`
- `upstream_api_accounts`
- `pool_quota_cycles`
- `account_pool_assignments`
- `upstream_capacity_reservations`
- `upstream_resource_owners`
- `quota_reconcile_runs`

额度字段统一使用 `BIGINT micro_credits`，当前约定 `1 credit = 1,000,000 micro_credits`。

## 帐号池配置示例

未配置 `upstream.pools` 时，启动时会按 `upstream.mode`、`openai.*`、`rust.*` 自动创建一个 `default` 池。需要多个池时可以显式配置：

```yaml
upstream:
  mode: "rust"
  pools:
    - pool_id: "rust-main"
      name: "rust-main"
      source_type: "rust"
      rust_grpc_addr: "127.0.0.1:50051"
      monthly_quota_credits: 1000000
      oversell_percent: 0.2
      exhaust_threshold: 0.98
      api_accounts:
        - account_ref: "reverse-1"
          monthly_quota_credits: 1000000
          priority: 100
    - pool_id: "openai-main"
      name: "openai-main"
      source_type: "normal"
      base_url: "https://api.openai.com"
      monthly_quota_credits: 1000000
      exhaust_threshold: 0.98
      api_accounts:
        - account_ref: "openai-1"
          api_key: "sk-..."
          monthly_quota_credits: 1000000
          priority: 100
```
