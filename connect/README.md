# Connect Layer

连接层，负责维护客户端长连接，实时推送消息。

## 核心亮点
- ⭐ Bucket分片机制（减少锁竞争）
- ⭐ CityHash分布算法（负载均衡）
- ⭐ Channel双向链表（高效会话管理）

## 职责
- WebSocket连接管理
- TCP连接管理（可选）
- 消息实时推送
- 用户在线状态管理