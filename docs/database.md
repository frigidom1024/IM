# 数据库设计文档

## ER 图

```mermaid
erDiagram
    user ||--o{ friend : "作为用户"
    user ||--o{ friend : "作为好友"
    user ||--o{ group : "创建"
    user ||--o{ group_member : "加入"
    user ||--o{ message : "发送"
    user ||--o{ message : "接收"
    user ||--o{ unread_count : "有未读"
    user ||--o{ unread_count : "来自"
    group ||--o{ group_member : "包含"
    group ||--o{ message : "群聊"
    group ||--o{ unread_count : "群未读"

    user {
        bigint id PK "用户ID"
        varchar username "用户名(唯一)"
        varchar password "密码(bcrypt)"
        varchar nickname "昵称"
        varchar avatar "头像URL"
        tinyint status "状态:0离线 1在线 2忙碌"
        timestamp created_at
        timestamp updated_at
    }

    friend {
        bigint id PK
        bigint user_id FK "用户ID"
        bigint friend_id FK "好友ID"
        varchar remark "备注"
        timestamp created_at
    }

    group {
        bigint id PK "群组ID"
        varchar name "群名称"
        varchar avatar "群头像"
        bigint owner_id FK "群主ID"
        int max_members "最大成员数"
        timestamp created_at
        timestamp updated_at
    }

    group_member {
        bigint id PK
        bigint group_id FK "群组ID"
        bigint user_id FK "用户ID"
        tinyint role "角色:0普通 1管理员 2群主"
        timestamp joined_at
    }

    message {
        bigint id PK "消息ID(snowflake)"
        bigint from_id FK "发送者ID"
        bigint to_id "接收者ID(0群消息)"
        bigint group_id "群组ID(0单聊)"
        tinyint msg_type "类型:1文字 2图片 3文件 4@提醒"
        text content "消息内容"
        tinyint is_revoked "是否撤回"
        timestamp created_at
    }

    unread_count {
        bigint user_id PK,FK "接收者ID"
        bigint from_id PK,FK "发送者ID(群聊0)"
        bigint group_id PK "群组ID(0私聊)"
        bigint unread_count "会话未读数量"
        bigint latest_message_id "最新消息ID"
        bigint latest_sender_id "最新消息发送者"
        varchar latest_message_type "最新消息类型"
        varchar latest_preview "最新消息预览"
        datetime latest_message_at "最新消息时间"
        datetime read_at "最后阅读时间"
    }
```

## 表结构说明

### 1. `user` — 用户表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 用户ID，主键自增 |
| username | VARCHAR(32) | 用户名，唯一索引 |
| password | VARCHAR(64) | bcrypt 加密密码 |
| nickname | VARCHAR(64) | 昵称 |
| avatar | VARCHAR(255) | 头像URL |
| status | TINYINT | 0离线 1在线 2忙碌 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

索引: `idx_status`, `idx_username`

---

### 2. `friend` — 好友关系表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键自增 |
| user_id | BIGINT | 用户ID |
| friend_id | BIGINT | 好友ID |
| remark | VARCHAR(64) | 好友备注 |
| created_at | TIMESTAMP | 添加时间 |

唯一约束: `uk_user_friend(user_id, friend_id)`
索引: `idx_user`

---

### 3. `group` — 群组表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 群组ID，主键自增 |
| name | VARCHAR(128) | 群名称 |
| avatar | VARCHAR(255) | 群头像 |
| owner_id | BIGINT | 群主ID |
| max_members | INT | 最大成员数，默认500 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

索引: `idx_owner`

---

### 4. `group_member` — 群成员表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键自增 |
| group_id | BIGINT | 群组ID |
| user_id | BIGINT | 用户ID |
| role | TINYINT | 0普通 1管理员 2群主 |
| joined_at | TIMESTAMP | 加入时间 |

唯一约束: `uk_group_user(group_id, user_id)`
索引: `idx_group`, `idx_user`

---

### 5. `message` — 消息表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 消息ID(snowflake生成) |
| from_id | BIGINT | 发送者ID |
| client_seq | VARCHAR(128) | 发送方幂等序列号，可为空 |
| to_id | BIGINT | 接收者ID(0表示群消息) |
| group_id | BIGINT | 群组ID(0表示单聊) |
| msg_type | TINYINT | 1文字 2图片 3文件 4@提醒 |
| content | TEXT | 消息内容 |
| is_revoked | TINYINT | 0正常 1已撤回 |
| created_at | TIMESTAMP | 发送时间 |

唯一约束: `uk_message_sender_seq(from_id, client_seq)`；索引: `idx_from`, `idx_to`, `idx_group`, `idx_created`

---

### 6. `unread_count` — 会话级未读聚合表

每个用户的每个私聊或群聊范围最多一条记录，不按单条消息创建未读记录。

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | BIGINT | 收件人用户ID |
| from_id | BIGINT | 私聊对方用户ID；群聊固定为0 |
| group_id | BIGINT | 群聊群组ID；私聊固定为0 |
| unread_count | BIGINT | 当前范围的未读总数 |
| latest_message_id | BIGINT | 最新消息ID |
| latest_sender_id | BIGINT | 最新消息真实发送者ID |
| latest_message_type | VARCHAR(20) | 最新消息类型 |
| latest_preview | VARCHAR(512) | 最新消息预览 |
| latest_message_at | DATETIME(3) | 最新消息时间 |
| read_at | DATETIME(3) | 最后阅读时间 |
| created_at | DATETIME(3) | 创建时间 |
| updated_at | DATETIME(3) | 更新时间 |

主键: `(user_id, from_id, group_id)`
私聊使用 `group_id=0`，群聊使用 `from_id=0`。

---

## 核心业务关系

```
用户 ── 好友关系 ── 其他用户
  │
  ├── 创建群组 ── 群组 ── 群成员 ── 其他用户
  │
  ├── 发送消息 ── 消息 ── 接收者/群组
  │
  └── 未读计数 ── 用户/群维度聚合
```

- **私聊**: `message.to_id != 0` 且 `message.group_id = 0`
- **群聊**: `message.group_id != 0` 且 `message.to_id = 0`
- **未读**: 按 `(user_id, from_id, group_id)` 聚合计数，读取时清零