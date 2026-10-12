# 数据库迁移

`im-migrate` 是独立部署命令；后端启动不会自动升级数据库。SQL 已打包进二进制，不需要随运行目录复制脚本。项目最低 Go 版本为 1.25.11，数据库为 MySQL 8.0。

## 命令与配置

```powershell
go run ./cmd/im-migrate -config internal/config/config-dev.yaml status
go run ./cmd/im-migrate -config internal/config/config-dev.yaml up
go run ./cmd/im-migrate -config internal/config/config-dev.yaml baseline -version 2
go run ./cmd/im-migrate -config internal/config/config-dev.yaml repair -version 4
```

`-config` 默认 `internal/config/config-dev.yaml`，只需要配置文件中的 `mysql.host`、`port`、`user`、`password`、`database`。无须配置 Redis、MinIO 或 JWT。目标数据库须预先创建；脚本在配置指定的库中执行，不会切换到硬编码的 `gochat`。密码放在受保护的配置文件，不放在命令行。

`status` 只读查询，不创建版本表。输出当前版本（`-1` 表示未登记）、dirty 状态、最新版本、待执行版本，以及未登记旧库的候选基线。候选版本只证明结构符合脚本，不能证明历史数据修复完整执行。结构不连续时候选为 `-1`，须先核对和修复结构。

`up` 顺序执行剩余迁移，已是最新版本时成功退出。版本记录存放在 `gochat_schema_migrations`。所有写操作使用同一个 MySQL 连接级迁移锁；并发执行只能串行进入，获取锁超时的命令失败退出。

## 空库初始化

在已创建、没有业务表的数据库执行 `up`。命令执行 `init.sql`，登记版本 2，再执行 003、004、005。初始化脚本已包含 001 的未读聚合结构和 002 的消息幂等字段，不重复执行这两个升级脚本。

Compose 仍通过 MySQL entrypoint 初始化新数据卷，顺序为 `init.sql`、003、004、005。初始化时由 `MYSQL_DATABASE` 指定库名。该方式不生成迁移记录；首次使用统一命令时执行 `status`，确认初始化和数据修复完成后登记 `baseline -version 5`。MySQL entrypoint 不会为已有数据卷重复执行这些 SQL。

## 旧库接入与升级

1. 备份数据库并验证恢复方式；停止旧应用写入，保留迁移前的应用版本。
2. 执行 `status`，核对已执行的 SQL 和候选基线。
3. 确认这些版本的数据修复也已完成，再执行 `baseline -version N`。
4. 执行 `up`，随后执行 `status`，确认版本 5 且 `dirty: false`。
5. 验证未读、群成员、好友和消息数据，再启动新应用。

| 基线 | 必须已完成的迁移 |
| --- | --- |
| 0 | 旧基础表；未读表使用 `count`，消息表尚无 `client_seq` |
| 1 | 001：会话未读聚合，旧表保留为 `unread_count_legacy` |
| 2 | 002：消息 `client_seq` 及发送者唯一索引；当前 `init.sql` 也属于该版本 |
| 3 | 003：会话偏好、上传会话、媒体资产和消息附件引用 |
| 4 | 004：群公告、成员昵称/禁言/已读游标，以及群角色修复 |
| 5 | 005：好友通知已读、私信额度和入群申请，以及历史私信额度回填 |

仅允许给无版本记录的数据库登记基线。检查覆盖表、列类型/长度及关键索引，拒绝部分升级或越序结构；不修改业务数据。`baseline` 命令表示操作者确认所选版本及之前的数据修复已经完成。

001 会合并同一用户群会话中来自多个发送者的未读行。004 按 `owner_id` 修复群角色并补齐群主成员。005 消耗已有历史私信方向的首次额度。升级前需要维护窗口，MySQL DDL 和大表回填可能持锁。

早期数字型 `message.msg_type` 的数据库不属于 001–005 支持的基线；须先完成历史字符串消息类型升级。混合或部分执行的脚本须核对后人工修复，命令不会自动猜测迁移状态。

## 失败与修复

初始化和每次迁移执行前记录 dirty，成功后清除。MySQL DDL 可能已提交部分修改，失败不代表所有语句都已回滚。失败后 `up` 和普通 `baseline` 都会拒绝继续。

Ctrl+C 取消当前命令；已开始执行的迁移保留 dirty，需要按同样流程检查和修复。

先执行 `status` 读取失败版本，查看该版本的 SQL，对照数据库确定已执行的语句。只人工补齐剩余语句，并核对数据修复结果；不要直接重跑含 `ALTER` 或 `RENAME` 的整个脚本。初始化失败记录为 dirty 版本 2，须补齐 `init.sql` 的所有基础表。

人工完成后执行 `repair -version N`，N 必须等于当前 dirty 版本。命令检查结构并清除 dirty，不执行数据修复；随后重新执行 `up`。`repair` 同样表示操作者已核实该版本的数据修复完成。不能用它回退版本、跳过迁移或改写健康版本。

无法确定已执行范围时恢复备份。工具不提供 `down`、删表或自动回滚；回退应用时保留新增表、列与未读旧表，不丢弃持久化数据。

## Docker

主镜像包含 `/app/im-migrate`。沿用服务配置挂载执行：

```powershell
docker compose -f gochat/docker-compose.yaml run --rm --no-deps gochat-server ./im-migrate -config internal/config/config-dev.yaml status
docker compose -f gochat/docker-compose.yaml run --rm --no-deps gochat-server ./im-migrate -config internal/config/config-dev.yaml baseline -version 5
docker compose -f gochat/docker-compose.yaml run --rm --no-deps gochat-server ./im-migrate -config internal/config/config-dev.yaml up
```

先启动数据库并确认就绪；首次升级前先停止旧后端写入。一次性命令不会自动启动其他 Compose 服务。若部署使用其他配置路径，相应替换 `-config`。

## 验证

默认 `go test ./...` 不需要数据库。隔离 MySQL 测试由 `GOCHAT_MIGRATION_TEST_DSN` 启用：将测试专用 MySQL 8.0 地址和凭据放入该环境变量，然后执行：

```powershell
go test -race -v ./internal/pkg/migration -count=1
```

测试只创建并删除自身生成的 `gochat_migrate_test_*` 数据库，不升级 DSN 中的已有数据库；测试账户需要建库/删库权限。用独立 MySQL 实例运行，避免使用部署账户。覆盖空库、历史数据升级、基线拒绝、dirty 修复、重复与并发升级，以及只读状态查询。
