# 数据表规划

```postgresql
-- Create user_credentials table
CREATE TABLE user_credentials (
    id BIGSERIAL PRIMARY KEY, -- 用户ID - 采用雪花ID
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL, -- 采用bcrypt算法
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
- 插入新纪录
- 修改 `password_hash`
- 修改 `status`

## 反设计模式

请生成对应的 `query.sql` 并使用 `sqlc` 生成代码，禁止手动展开或使用 ORM 框架