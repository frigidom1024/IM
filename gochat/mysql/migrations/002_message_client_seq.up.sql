-- 002: 为消息增加发送方幂等序列号。
-- 相同发送者的相同 client_seq 只能对应一条消息；NULL 表示旧客户端或未启用幂等。

ALTER TABLE `message`
  ADD COLUMN `client_seq` VARCHAR(128) NULL COMMENT '发送方幂等序列号' AFTER `from_id`,
  ADD UNIQUE KEY `uk_message_sender_seq` (`from_id`, `client_seq`);