CREATE DATABASE IF NOT EXISTS gochat DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE gochat;

-- 用户表
CREATE TABLE IF NOT EXISTS `user` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '用户ID',
  `username` VARCHAR(32) NOT NULL UNIQUE COMMENT '用户名',
  `password` VARCHAR(64) NOT NULL COMMENT '密码(bcrypt)',
  `nickname` VARCHAR(64) DEFAULT '' COMMENT '昵称',
  `avatar` VARCHAR(255) DEFAULT '' COMMENT '头像URL',
  `status` TINYINT DEFAULT 0 COMMENT '状态:0离线 1在线 2忙碌',
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_status(`status`),
  INDEX idx_username(`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- 好友关系表
CREATE TABLE IF NOT EXISTS `friend` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL COMMENT '用户ID',
  `friend_id` BIGINT NOT NULL COMMENT '好友ID',
  `remark` VARCHAR(64) DEFAULT '' COMMENT '好友备注',
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_friend(`user_id`, `friend_id`),
  INDEX idx_user(`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='好友表';

-- 群组表
CREATE TABLE IF NOT EXISTS `group` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '群组ID',
  `name` VARCHAR(128) NOT NULL COMMENT '群名称',
  `avatar` VARCHAR(255) DEFAULT '' COMMENT '群头像',
  `owner_id` BIGINT NOT NULL COMMENT '群主ID',
  `max_members` INT DEFAULT 500 COMMENT '最大成员数',
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_owner(`owner_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='群组表';

-- 群成员表
CREATE TABLE IF NOT EXISTS `group_member` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `group_id` BIGINT NOT NULL COMMENT '群组ID',
  `user_id` BIGINT NOT NULL COMMENT '用户ID',
  `role` TINYINT DEFAULT 0 COMMENT '角色:0普通 1管理员 2群主',
  `joined_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_group_user(`group_id`, `user_id`),
  INDEX idx_group(`group_id`),
  INDEX idx_user(`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='群成员表';

-- 消息表
CREATE TABLE IF NOT EXISTS `message` (
  `id` BIGINT PRIMARY KEY COMMENT '消息ID(snowflake)',
  `from_id` BIGINT NOT NULL COMMENT '发送者ID',
  `to_id` BIGINT DEFAULT 0 COMMENT '接收者ID(0表示群消息)',
  `group_id` BIGINT DEFAULT 0 COMMENT '群组ID(0表示单聊)',
  `msg_type` VARCHAR(20) NOT NULL COMMENT '类型:text/image/file/voice/video/mention',
  `content` TEXT NOT NULL COMMENT '消息内容(文字原文/多媒体FileMeta JSON)',
  `is_revoked` TINYINT DEFAULT 0 COMMENT '是否撤回:0否 1是',
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_from(`from_id`),
  INDEX idx_to(`to_id`),
  INDEX idx_group(`group_id`),
  INDEX idx_created(`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息表';

-- 未读计数表（聚合存储每个用户对每个发送者/群组的未读消息数量）
CREATE TABLE IF NOT EXISTS `unread_count` (
  `user_id` BIGINT NOT NULL COMMENT '接收者ID',
  `from_id` BIGINT NOT NULL COMMENT '发送者ID（群聊时为0）',
  `group_id` BIGINT DEFAULT 0 COMMENT '群组ID（0表示私聊）',
  `count` BIGINT DEFAULT 0 COMMENT '未读数量',
  `read_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`user_id`, `from_id`, `group_id`),
  INDEX idx_user(`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='未读计数表';
