# 用户登录与 Session

当前用户域位于：

- `internal/domain/user/entity`
- `internal/domain/user/repository`
- `internal/domain/user/service`

完整 MySQL 建表 SQL 见 `docs/mysql-schema.sql`。

## 数据表

### `users`

存储登录用户。

核心字段：

- `username`：登录用户名。
- `password_hash`：bcrypt 密码哈希。
- `display_name`：展示名。
- `role`：角色。
- `enabled`：是否启用。

### `user_sessions`

存储登录 session。

核心字段：

- `user_id`：用户 ID。
- `token_hash`：session token 的 SHA-256 哈希。
- `expires_at`：过期时间。
- `revoked_at`：退出登录时间。

Cookie 中保存的是明文随机 token，数据库只保存 token hash。

## 接口

### 登录

```http
POST /auth/login
Content-Type: application/json
```

```json
{
  "username": "admin",
  "password": "admin123456"
}
```

登录成功后会写入 session cookie。

### 当前用户

```http
GET /auth/me
```

通过 session cookie 返回当前用户。

### 退出

```http
POST /auth/logout
```

注销当前 session，并清理 cookie。
