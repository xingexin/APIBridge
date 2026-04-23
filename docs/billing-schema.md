# 计费数据库设计

当前计费按账号维度扣费，API Key 只是账号的访问凭证。数据库默认使用 MySQL，并通过 GORM 自动迁移表结构。

默认 DSN 在 `cfg/config.yaml` 的 `database.dsn` 中配置。

wallet 域内的实体位于 `internal/domain/wallet/entity`，仓库操作位于 `internal/domain/wallet/repository`。MySQL 连接在服务启动时创建，并注入 wallet 仓库。

完整 MySQL 建表 SQL 见 `docs/mysql-schema.sql`。

## 表结构

### `wallet_accounts`

存储计费账号和余额。

核心字段：

- `account_id`：业务账号 ID，配置初始化时使用。
- `name`：账号名称。
- `balance`：账号余额。
- `enabled`：账号是否启用。

### `wallet_api_keys`

存储账号下的平台 API Key。

核心字段：

- `account_id`：关联 `wallet_accounts.id`。
- `key`：客户端请求时使用的 API Key。
- `name`：Key 名称。
- `enabled`：Key 是否启用。

一个账号可以绑定多个 API Key，所有 Key 共用账号余额。

### `wallet_usage_records`

存储每次请求的用量和扣费记录。

核心字段：

- `account_id`：关联账号。
- `api_key_id`：本次请求使用的 API Key。
- `model`：模型名。
- `input_tokens`：输入 token。
- `output_tokens`：输出 token。
- `total_tokens`：总 token。
- `cost`：本次费用。
- `balance_after`：扣费后余额。
- `trace_id`：链路追踪 ID。

## 扣费流程

1. 客户端请求 `/v1/*`，携带 `Authorization: Bearer <平台 API Key>`。
2. Go 根据 API Key 查询 `wallet_api_keys` 和对应账号。
3. 账号或 Key 禁用、余额不足时拒绝请求。
4. 请求转发到上游。
5. 上游响应结束后解析 `usage`。
6. 根据模型单价计算费用。
7. 在数据库事务中扣减账号余额，并写入 `wallet_usage_records`。
