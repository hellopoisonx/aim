# 需求到模块映射

## 需求关键词 -> 主域

- “登录、token、注册、退出登录、注销、刷新、管理员、鉴权” -> `aim-auth-domain`
- “网关、REST API、http、ws、websocket” -> `aim-gateway-domain`
- “用户信息、逻辑、logic” -> `aim-logic-domain`
- “核心、core、转发” -> `aim-core-domain`
- “dev-tool、aim_test、冒烟测试、集成测试、接口调试、测试脚本” -> `aim-dev-tool`

## 不要误判

- ”refresh token 和 access token 的签发属于 auth 域， 但是 access token （jwt） 的快速验签发生在 gateway 域“
- ”只要涉及到 REST API， 一定归属于 gateway 域， 其他域只暴露内部grpc调用接口， 由gateway代理转发“