# 模块需求定义

独立的认证服务，位于网关之后、业务服务之前。

- JWT 签发与刷新
- 多设备登录策略（单设备踢下线 / 多设备共存）
- 复杂鉴权

## AccessToken

- JWT Token
- 委托 `gateway` 进行本地快速验签
- 无状态
- TTL: 5 min

## RefreshToken

- UUID
- 由 `auth` 管理
- TTL: 7 day
- 有状态(redis)
  - `auth:rt:{token}`: `{user_id}:{device_id}` ttl: 7 days
  - `auth:device:{user_id}:{device_id}`: `{token}` no ttl