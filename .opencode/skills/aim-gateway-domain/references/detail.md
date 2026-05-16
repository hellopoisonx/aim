# 模块需求定义

维护长连接（WebSocket / TCP）、Protobuf 协议解析、心跳保活、本地快速验证 JWT。

- 有状态服务，根据 User_ID 做一致性哈希，确保同一用户落在固定网关节点
- 使用 150+ 虚拟节点/物理节点，减少 rebalancing 影响
- 节点下线前推送 reconnect 帧，提供 5-10s drain 窗口，避免惊群重连