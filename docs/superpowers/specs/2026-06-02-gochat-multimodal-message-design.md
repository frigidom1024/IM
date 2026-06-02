# GoChat 多模态消息系统设计

## 概述

为 GoChat 添加通用多模态消息能力，支持图片、文件、语音、视频等消息类型。系统设计为可扩展的架构，新增消息类型无需改表结构。

## 设计原则

- **向后兼容**：现有文字消息不受影响
- **自描述协议**：消息类型使用字符串而非魔法数字
- **统一 Content 字段**：所有消息类型的载荷统一存储在 `Content` 字段，文字存原文，多媒体存 JSON
- **扩展友好**：新增消息类型只需加一个枚举值 + 前端渲染组件，后端无代码变更

## 消息类型枚举

字符串标识替代原有的 int8，协议自描述：

| 常量 | 值 | 说明 |
|------|-----|------|
| MsgTypeText | `"text"` | 文字消息 |
| MsgTypeImage | `"image"` | 图片消息 |
| MsgTypeFile | `"file"` | 文件消息 |
| MsgTypeMention | `"mention"` | @提醒消息 |
| MsgTypeVoice | `"voice"` | 语音消息 |
| MsgTypeVideo | `"video"` | 视频消息 |

## Content JSON 格式

### text

```json
{"text": "你好"}
```

### image

```json
{
  "url": "/minio/images/2026/06/abc123.jpg",
  "thumbnail": "/minio/thumbnails/2026/06/abc123_thumb.jpg",
  "width": 1920,
  "height": 1080,
  "size": 256000
}
```

### file

```json
{
  "url": "/minio/files/2026/06/abc123.pdf",
  "name": "报告.pdf",
  "size": 2048000,
  "mime": "application/pdf"
}
```

### voice

```json
{
  "url": "/minio/voices/2026/06/abc123.mp3",
  "duration": 15.5,
  "size": 51200
}
```

### video

```json
{
  "url": "/minio/videos/2026/06/abc123.mp4",
  "thumbnail": "/minio/thumbnails/2026/06/abc123_thumb.jpg",
  "duration": 120.0,
  "size": 50000000
}
```

## 上传流程

后端代理上传模式：

```
前端 POST /api/upload (multipart/form-data)
  form: file + type
  
后端 UploadHandler:
  1. 按 msgType 校验文件大小和 MIME
  2. 生成唯一文件名 (uuid)
  3. 上传到 MinIO（按类型分区）
  4. 如果是图片，生成缩略图（可选）
  5. 返回 FileMeta JSON

前端收到 FileMeta 后，通过 WebSocket 发送消息:
  { cmd: "msg", data: { type: "image", content: '<FileMeta JSON>' } }
```

## MinIO 客户端

新建 `server/pkg/minio/client.go`：

```go
type Client struct {
    client *minio.Client
    bucket string
}

type FileMeta struct {
    URL       string  `json:"url"`
    Name      string  `json:"name"`
    Size      int64   `json:"size"`
    MIME      string  `json:"mime"`
    Thumbnail string  `json:"thumbnail,omitempty"`
    Width     int     `json:"width,omitempty"`
    Height    int     `json:"height,omitempty"`
    Duration  float64 `json:"duration,omitempty"`
}

func (c *Client) Upload(ctx, path string, reader io.Reader, size int64, contentType string) (*FileMeta, error)
func (c *Client) Delete(ctx, path string) error
func (c *Client) PresignedURL(ctx, path string, expires time.Duration) (string, error)
```

对象路径规则：

```
images/{year}/{month}/{uuid}.{ext}
files/{year}/{month}/{uuid}.{ext}
voices/{year}/{month}/{uuid}.{ext}
videos/{year}/{month}/{uuid}.{ext}
thumbnails/{year}/{month}/{uuid}_thumb.{ext}
```

## 文件校验规则

| 类型 | 最大尺寸 | 允许的 MIME |
|------|----------|-------------|
| image | 10MB | image/jpeg, image/png, image/gif, image/webp |
| file | 100MB | application/pdf, application/zip, application/gzip, text/plain, application/msword, application/vnd.openxmlformats-officedocument.wordprocessingml.document |
| voice | 10MB | audio/mp3, audio/wav, audio/ogg, audio/aac |
| video | 200MB | video/mp4, video/webm, video/quicktime |

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/upload` | 上传文件（需认证） |
| GET | `/api/files/:objectPath` | 获取文件（presigned URL 重定向） |

对象路径中的 `/` 使用 URL 编码传递。

## 文件访问

FileMeta.URL 返回 MinIO 上存储的对象路径（如 `images/2026/06/uuid.jpg`），文件读取有以下两种方式：

- **开发环境**: MinIO 控制台直连查看
- **生产环境**: 通过 `GET /api/files/{encoded_path}` 后端代理或 presigned URL 302 重定向，确保访问鉴权

## 数据库迁移

```sql
ALTER TABLE `message` MODIFY COLUMN `msg_type` VARCHAR(20) NOT NULL;
UPDATE `message` SET `msg_type` = 'text'    WHERE `msg_type` = '1';
UPDATE `message` SET `msg_type` = 'image'   WHERE `msg_type` = '2';
UPDATE `message` SET `msg_type` = 'file'    WHERE `msg_type` = '3';
UPDATE `message` SET `msg_type` = 'mention' WHERE `msg_type` = '4';
```

## 影响文件清单

### Phase 1: MinIO 客户端 + 上传 API

| 文件 | 操作 |
|------|------|
| `server/pkg/minio/client.go` | 新建 — MinIO 客户端封装 |
| `server/pkg/minio/upload.go` | 新建 — 上传逻辑 + 文件校验 |
| `server/api/handler/upload.go` | 新建 — UploadHandler |
| `server/api/router.go` | 修改 — 注册 upload 路由 |
| `server/main.go` | 修改 — 初始化 MinIO 客户端 |

### Phase 2: 消息模型升级

| 文件 | 操作 |
|------|------|
| `server/logic/model/message.go` | 修改 — 常量 + MsgType 改 string |
| `server/connect/types.go` | 修改 — ChatMsgData.Type 改 string, Validate 更新 |
| `server/connect/handler/chat.go` | 修改 — 适配 string type |
| `server/connect/errors.go` | 修改 — ErrInvalidMsgType 文案 |
| `docker/init.sql` | 修改 — msg_type VARCHAR(20) |
| `deploy/mysql/init.sql` | 修改 — 同上 |

### Phase 3: WebSocket 集成

| 文件 | 操作 |
|------|------|
| `server/connect/msg.go` | 修改 — ParseWsMsg 增加 CmdMsg 的 Data 解析 |

### Phase 4: 前端组件

| 文件 | 操作 |
|------|------|
| `web/src/api/upload.js` | 新建 — 上传 API 封装 |
| `web/src/components/ImageMessage.vue` | 新建 — 图片消息渲染 |
| `web/src/components/FileMessage.vue` | 新建 — 文件消息渲染 |
| `web/src/components/VoiceMessage.vue` | 新建 — 语音消息渲染 |

## 实施顺序

```
Phase 1: MinIO 客户端 + 上传 API
Phase 2: 消息模型升级（含 DB 迁移）
Phase 3: WebSocket 收发集成
Phase 4: 前端组件
```

每个 Phase 独立可测试，Phase 1 完成后即可手动上传文件验证 MinIO 集成，Phase 2 完成后文字和多媒体消息可混发。
