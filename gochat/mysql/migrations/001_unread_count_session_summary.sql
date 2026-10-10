-- 001: 将旧的未读计数表升级为会话级未读摘要表。
-- 说明：旧群聊记录可能按 from_id 分散，需要先按 user_id + group_id 聚合。
-- 执行前请备份数据库，并在低流量窗口运行。

CREATE TABLE `unread_count_v2` (
  `user_id` BIGINT NOT NULL,
  `from_id` BIGINT NOT NULL DEFAULT 0,
  `group_id` BIGINT NOT NULL DEFAULT 0,
  `unread_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `latest_message_id` BIGINT DEFAULT NULL,
  `latest_sender_id` BIGINT DEFAULT NULL,
  `latest_message_type` VARCHAR(20) NOT NULL DEFAULT 'text',
  `latest_preview` VARCHAR(512) NOT NULL DEFAULT '',
  `latest_message_at` DATETIME(3) DEFAULT NULL,
  `read_at` DATETIME(3) DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`user_id`, `from_id`, `group_id`),
  INDEX idx_unread_user_latest (`user_id`, `latest_message_at`),
  INDEX idx_unread_user_count (`user_id`, `unread_count`),
  CONSTRAINT chk_unread_scope_v2 CHECK (
    (`group_id` = 0 AND `from_id` > 0)
    OR (`group_id` > 0 AND `from_id` = 0)
  ),
  CONSTRAINT chk_unread_count_non_negative_v2 CHECK (`unread_count` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户会话未读聚合表';

INSERT INTO `unread_count_v2` (
  `user_id`, `from_id`, `group_id`, `unread_count`,
  `latest_message_type`, `read_at`
)
SELECT
  `user_id`,
  `from_id`,
  0,
  `count`,
  'text',
  `read_at`
FROM `unread_count`
WHERE `group_id` = 0;

INSERT INTO `unread_count_v2` (
  `user_id`, `from_id`, `group_id`, `unread_count`,
  `latest_message_type`, `read_at`
)
SELECT
  `user_id`,
  0,
  `group_id`,
  SUM(`count`),
  'text',
  MAX(`read_at`)
FROM `unread_count`
WHERE `group_id` > 0
GROUP BY `user_id`, `group_id`;

RENAME TABLE `unread_count` TO `unread_count_legacy`,
             `unread_count_v2` TO `unread_count`;

-- 迁移后确认数据无误，再手动执行：
-- DROP TABLE `unread_count_legacy`;