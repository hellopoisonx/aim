# 模块路由

## 模块职责

- `auth`：登录态、`RefreshToken`、 `AccessToken` 、 刷新 tokens。
- `gateway`：面向客户端的http网关，负责维持管理 `ws` 连接并转发请求给下游grpc服务。
- `core`: 负责转发消息。
- `logic`：逻辑判断层。

## 交接信号

- 需求同时涉及一个域的 API 和另一个域的配置，说明已经越过总路由层，应该切到对应领域 Skill。