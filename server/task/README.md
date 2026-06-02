# Task Layer

任务层，消费消息队列，异步处理消息。

## 职责
- 从Redis队列消费消息
- 消息持久化到MySQL
- 调用Connect层推送消息
- 离线消息处理