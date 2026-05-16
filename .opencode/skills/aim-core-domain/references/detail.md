# 模块需求定义

只负责一件事：把消息送到对的人。

- Transfer Service（消息路由）：消息流向判断（单聊/群聊）、查询接收方所在网关节点、投递至 Kafka
- Presence Service（在线状态）：Redis heartbeat 维护用户在线/离线/输入中状态，向好友推送状态变更
- Delivery Consumer（投递消费者）：从 Kafka 消费消息，查找目标用户所在网关并投递