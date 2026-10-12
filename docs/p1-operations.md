# P1 集成与部署

P1 保持单实例架构，增加 Go SDK 产品化、运维指标、OpenAPI、会话、群管理和媒体能力。HTTP 契约以 [OpenAPI](api/openapi.json) 为准；WebSocket 使用 [协议文档](websocket.md)，Go 集成使用 [SDK 文档](../sdk/README.md)。

## 数据库升级

新 Compose 数据卷依次执行 init.sql、003、004。已有数据库先备份，确认 P0 的 001/002 已执行，再依次执行：

```powershell
Get-Content gochat/mysql/migrations/003_p1_conversations_media.sql | docker exec -i <mysql-container> mysql --defaults-extra-file=/run/secrets/mysql-client.cnf <database>
Get-Content gochat/mysql/migrations/004_p1_groups.sql | docker exec -i <mysql-container> mysql --defaults-extra-file=/run/secrets/mysql-client.cnf <database>
Get-Content gochat/mysql/migrations/005_relationships_applications.sql | docker exec -i <mysql-container> mysql --defaults-extra-file=/run/secrets/mysql-client.cnf <database>
```

004 的 ALTER 只能执行一次，先检查 `group.announcement` 和 `group_member.nickname` 等列。大表 ALTER 应在维护窗口执行。003 新增会话偏好、上传会话、资产和可信消息附件引用表。004 增加群公告、成员昵称、禁言和已读游标，并以 owner_id 修复旧角色：普通成员=0、管理员=1、群主=2。无法可靠区分的旧角色不会自动提升管理员，需要群主重新授权。

回退应用时保留新增表与列；删除它们会丢失草稿、上传进度、文件授权和群管理状态。旧消息 URL 不会被自动转换为可信附件分享；新附件在消息持久化时验证权限并写入引用表。

上述命令要求预先挂载只读 MySQL 客户端配置文件，包含部署账户及密码；不要将密码写入命令行或仓库。

## 会话与群管理

`GET /api/conversation?limit=50&offset=0` 返回有消息或已主动打开的私信与群聊。好友、群成员关系本身不创建空会话；`POST /api/conversation/:scope/open` 持久化打开状态且保留草稿。以 `private:<user_id>` 或 `group:<group_id>` 标识，置顶、免打扰和草稿只影响当前用户。PATCH 使用可选字段，传 false 或空字符串可以清除偏好。

005 为好友增加接收方通知已读时间，新增永久保留的私信额度与入群申请记录。升级时历史已有发送记录的方向视为首次额度已使用，互加好友正常发送不受影响。005 的 ALTER 只执行一次；先备份数据库并停止旧应用写入，再迁移并启动新后端与前端。旧应用不执行新权限规则，不应在迁移后继续提供写入服务。

单向添加允许一条文本，互加后支持所有现有消息类型。删除、重加、撤回不恢复额度。额度、好友关系变更和发送事务在同一用户对上串行化；WS 错误码 42010/42011/42012 分别表示未添加、只能发文本、首次额度已使用。

主动入群的 `/join` 现在返回待审核申请，不直接授予成员身份。群主和管理员邀请保持立即入群。只有当前群主能批准或拒绝；满员批准返回409并保留申请。Go SDK 使用 `ApplyToGroup` 返回申请，旧 `JoinGroup` 保留签名但返回 `ErrGroupApprovalPending`，避免误认为已入群。`friend_changed`、`group_changed` 是事务提交后的刷新通知，客户端在登录和重连时重新查询持久化状态。

群管理提供搜索、邀请、公告、昵称、禁言、角色管理、转让和解散。管理员不能修改群主或同级管理员；群主退出前必须转让或解散。群成员列表及回执支持 limit/offset。群禁言与成员身份在入库事务中再次校验。`message/read` 的群 message_id 是已读游标，可使用十进制字符串避免 JavaScript 整数精度问题；省略时清除当前群会话未读。

客户端免打扰由宿主 UI 使用；服务仍正常投递消息和累计未读。群文件列表包含直接归属该群的上传及通过可信群消息分享的附件，按资产去重。

## 上传与媒体处理

原 multipart `POST /api/upload` 继续可用，可额外传 group_id。新响应的 url/thumbnail 指向受 JWT 保护的 HTTP 路由。浏览器应带 Authorization 获取 Blob，再创建本地 object URL；不要直接将受保护 URL 放入没有认证 header 的 img/audio/video 标签。Go SDK 的 DownloadFile 返回流，调用方必须关闭 Body。

断点续传流程：POST `/api/upload/session` 声明 name/type/mime/size；PATCH `/api/upload/session/:id` 发送最多 8MiB 的原始字节并提供 Upload-Offset；GET 查询服务端已确认的 offset；最后 POST `/:id/complete` 获取 FileMeta。偏移冲突返回409；完成操作幂等；DELETE 可取消未完成会话。网络结果不明确时先查询偏移，不能盲目重复分块。SDK UploadFile 返回错误时保留 SessionID，供调用者显式恢复。

图片/语音最大50MiB、普通文件500MiB、视频1GiB；图片最大2000万像素。媒体处理最多两个并发任务、两分钟、两个编码线程，视频输出最多1GiB、30fps；音视频时长最多一小时。支持 JPEG/PNG/GIF/WebP 缩略图、音频时长、视频 H.264/AAC MP4 转码。视频/WebP/音频依赖 PATH 中的 ffmpeg/ffprobe，Docker 镜像已安装。使用明确 demuxer 和 file/pipe 协议，不加载客户端文件名或外部播放列表。

上传会话24小时过期。将 GOCHAT_UPLOAD_DIR 挂载为持久化目录，并保持重启后的路径相同；Compose 使用 `/app/uploads` 卷。磁盘容量至少满足活跃上传与转码临时空间。该方案保证单实例服务重启后的续传，不提供多个实例共享临时文件的能力。

`expires_hours` 可选指定资产过期时间，默认永久。下载检查所有者、当前群成员和已验证消息参与者；过期返回410，清理后404。文件权限不能通过任意消息文本获得。

## 过期清理与对象存储

使用同一配置、数据库和上传目录周期运行维护命令，每次清理最多100个过期会话、100个资产及100个超过24小时且无数据库记录的孤儿对象。对象列表流式扫描，合法永久资产不占删除额度：

```powershell
go run ./cmd/im-maintenance -config internal/config/config-dev.yaml
```

Docker 中可执行 `docker compose exec gochat-server ./im-maintenance`。部署时可将命令交给现有调度系统；数量超过100时重复执行。首次配置桶生命周期可以添加 `-lifecycle-days 7`，只对 staging/ 和该范围内未完成的 S3 multipart 生效；永久聊天附件由数据库 expiry 和维护任务管理。该选项会替换桶生命周期配置，已有桶规则需要先备份并合并后再应用。对象存储需要兼容 Put/Get/Head/Delete、Range 和 S3 lifecycle。

Docker E2E 使用本地 S3 HTTP 测试替身验证对象字节、SDK、下载和权限，不验证 AWS 签名及真实 MinIO 生命周期调度；替身仅支持小文件，不用于生产。真实桶的生命周期策略应在部署目标上验证。

运维健康检查和 Prometheus 配置见 [runtime](operations/runtime.md)。
