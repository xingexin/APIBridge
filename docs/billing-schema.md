# 计费数据库设计

当前计费已经拆到 `internal/domain/billing`，只负责客户侧额度、充值周期、价格快照、预占、提交、释放和账本。上游帐号池容量在 `internal/domain/upstream`，完整链路见 `docs/mvp-architecture.md`。

数据库默认使用 MySQL，并通过 GORM 自动迁移表结构。默认 DSN 在 `cfg/config.yaml` 的 `database.dsn` 中配置。

## Billing 表

### `billing_accounts`

客户代理帐号。平台 API Key 会解析到该帐号。

核心字段：

- `account_id`：业务账号 ID。
- `name`：账号名称。
- `enabled`：账号是否启用。
- `current_period_id`：当前客户额度周期。

### `billing_api_keys`

客户访问 GPTBridge 的平台 API Key。

核心字段：

- `account_id`：关联 `billing_accounts.id`。
- `key`：客户端请求时使用的 API Key。
- `name`：Key 名称。
- `enabled`：Key 是否启用。

### `account_quota_periods`

客户充值周期账本。周期到期后新建或激活下一周期，不覆盖历史。

核心字段：

- `quota_micro_credits`：本周期总额度。
- `used_micro_credits`：已提交用量。
- `reserved_micro_credits`：请求中预占额度。
- `period_start_at` / `period_end_at`：周期时间。
- `status`：`active`、`pending`、`expired`。

### `billing_reservations`

客户侧请求预占记录。

核心字段：

- `reservation_id`：幂等提交/释放 ID。
- `request_id` / `trace_id`：链路标识。
- `endpoint`、`model`、`settlement_policy`、`policy_version`。
- `price_snapshot`：请求发生时的价格快照。
- `estimated_micro_credits`、`reserved_micro_credits`、`final_micro_credits`。
- `status`：`reserved`、`committed`、`released`。
- `expires_at`：后续清理过期预占使用。

### `billing_ledger_records`

客户额度流水。每次 reserve、commit、release 都会写入一条记录，便于审计和对账。

## 请求结算流程

1. Gateway 解析平台 API Key，得到客户帐号。
2. Tx1 中预占客户额度。
3. 上游请求结束后：
   - 成功且有 usage：按真实 usage 提交，多退少补。
   - 成功但无 usage：按 endpoint policy 提交预估值、最低费用或释放。
   - 上游失败、客户端取消、流式复制失败：释放预占。
4. Commit/Release 都按 `reservation_id` 幂等处理。

