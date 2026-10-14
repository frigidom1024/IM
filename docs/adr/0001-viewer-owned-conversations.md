# 会话状态由每位用户独立拥有

私信双方与各个群成员需要独立的未读、阅读时间和偏好，因此 `conversation` 使用 `(from_id,to_id,group_id)` 主键，`from_id` 是拥有者。消息由多位用户的会话视图共同引用，不增加 `message.conversation_id`；MessageService 处理单条消息，ConversationService 管理会话范围，MessagingService 在同一事务内协调消息和会话投影，并在提交后提供投递目标。

已读表示清除整个会话当前的未读数，仅通过 HTTP 会话接口执行。取消 WS `msg_read`、单消息游标和群已读回执，以免存在两套阅读状态；旧表和旧群成员字段仅作为迁移备份保留。升级 006 需要停止旧应用写入，完成回填后再运行新版本。
