# 数据表规划

```postgresql
-- Create user_credentials table
CREATE TABLE user_credentials (
    id BIGINT PRIMARY KEY, -- 用户ID - 采用雪花ID
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL, -- 采用bcrypt算法
    name VARCHAR(255) NOT NULL, -- 用户名/昵称，注册必填
    status SMALLINT NOT NULL DEFAULT 1, -- 用户状态： 1（正常、默认） 0（封禁）
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for fast lookups
CREATE INDEX idx_user_credentials_email ON user_credentials (email);
```

## 权限

只由 `auth` 模块持有

## 操作

- 按 `id` 查询
- 按 `email` 查询
- 插入新纪录：由 `app/shared/tools.Snowflake` 在 auth 服务侧生成 `BIGINT` 雪花 ID，并显式传入 `CreateUser`；`name` 必填且数据库约束为 `NOT NULL`
- 修改 `password_hash`
- 修改 `status`

## 反设计模式

请生成对应的 `query.sql` 并使用 `sqlc` 生成代码，禁止手动展开或使用 ORM 框架

## 当前实现

- schema：`app/auth/rpc/model/migrations/000_init.sql`
- query：`app/auth/rpc/model/queries/auth.sql`
- sqlc 配置：`app/auth/rpc/model/sqlc.yaml`
- 生成包：`app/auth/rpc/model`

重新生成：

```bash
cd app/auth/rpc/model
sqlc generate
```

当前查询覆盖：

- `CreateUser`：参数包含 `id`、`email`、`password_hash`、`name`，不要依赖数据库自增或默认值
- `GetUserByEmail`
- `GetUserByID`
- `UpdatePassword`
- `UpdateStatus`

## 雪花 ID

- 生成器：`app/shared/tools/snowflake.go`
- auth 注入点：`app/auth/rpc/internal/service.NewSQLUserStoreWithMachineID`
- 配置项：`Auth.SnowflakeMachineID`，多实例部署时每个 auth 实例必须使用不同 machine ID，避免 ID 冲突
