# GoChat 微服务化与前端组件化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 GoChat 改造为可被外部系统集成的微服务（后端）+ 可嵌入的 Vue 组件（前端）

**架构：** GoChat 不实现用户认证，启动配置 `auth.mode: external` 后，认证时调用外部系统 `GET /user/info` 获取用户信息。ChatWidget 接收外部 token，自动完成认证和 WS 连接

**Tech Stack:** Go 1.25, Gin, GORM, Vue 3, Vite

---

## 文件清单

### 新建文件

| 文件 | 职责 |
|------|------|
| `server/api/handler/auth_external.go` | 外部认证处理器（调外部 API） |
| `web/src/components/ChatWidget.vue` | 可嵌入的核心聊天组件 |
| `web/src/composables/useChat.js` | 封装的聊天逻辑（WS连接/消息状态） |

### 修改文件

| 文件 | 变更 |
|------|------|
| `server/pkg/config/config.go` | 增加 `Auth.Mode` 和 `Auth.ExternalUserAPI` 配置 |
| `server/config/config-dev.yaml` | 添加 auth 配置项 |
| `server/api/router.go` | 添加 `/api/auth` 端点 + `/healthz` |
| `server/main.go` | 认证方式选择 |
| `web/src/stores/user.js` | 支持外部认证流程 |
| `gochat/docker-compose.yaml` | 添加 AUTH_MODE 环境变量 |

---

### Task 1: 添加 auth 配置项

**Files:**
- Modify: `server/pkg/config/config.go`
- Modify: `server/config/config-dev.yaml`

- [ ] **Step 1: 更新 Config 结构体**

```go
// AuthConfig 认证配置
type AuthConfig struct {
	Mode            string `yaml:"mode"`              // builtin | external
	ExternalUserAPI string `yaml:"external_user_api"` // 外部用户信息API
}

type Config struct {
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	MinIO  MinIOConfig  `yaml:"minio"`
	JWT    JWTConfig    `yaml:"jwt"`
	Logger LoggerConfig `yaml:"logger"`
}
```

- [ ] **Step 2: 更新 config-dev.yaml**

```yaml
auth:
  mode: "builtin"
  external_user_api: ""
```

- [ ] **Step 3: 提交**

```bash
git add server/pkg/config/config.go server/config/config-dev.yaml
git commit -m "feat: 添加 auth 配置项支持 external 模式"
```

---

### Task 2: 添加 auth 配置项

**Files:**
- Modify: `server/pkg/config/config.go`
- Modify: `server/config/config-dev.yaml`
- Modify: `gochat/docker-compose.yaml`
- Modify: `deploy/server/config-dev.yaml`

- [ ] **Step 1: 更新 Config 结构体**

```go
// AuthConfig 认证配置
type AuthConfig struct {
	Mode            string `yaml:"mode"`              // builtin | external
	ExternalUserAPI string `yaml:"external_user_api"` // 外部用户信息API
}

type Config struct {
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	MinIO  MinIOConfig  `yaml:"minio"`
	JWT    JWTConfig    `yaml:"jwt"`
	Logger LoggerConfig `yaml:"logger"`
}
```

- [ ] **Step 2: 更新 config-dev.yaml**

```yaml
auth:
  mode: "builtin"
  external_user_api: ""
```

- [ ] **Step 3: 提交**

```bash
git add server/pkg/config/config.go server/config/config-dev.yaml
git commit -m "feat: 添加 auth 配置项支持 external 模式"
```

---

### Task 3: 实现外部认证处理器

**Files:**
- Create: `server/api/handler/auth_external.go`

- [ ] **Step 1: 创建 auth_external.go**

```go
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gochat/logic/model"
	"gochat/logic/registry"
	"gochat/pkg/errors"
	"gochat/pkg/logger"
	"gochat/pkg/util"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ExternalAuthHandler 外部认证处理器
type ExternalAuthHandler struct {
	userRepo  registry.UserRepository
	apiURL    string // 外部用户信息API地址
	httpClient *http.Client
}

// NewExternalAuthHandler 创建外部认证处理器
func NewExternalAuthHandler(userRepo registry.UserRepository, apiURL string) *ExternalAuthHandler {
	return &ExternalAuthHandler{
		userRepo:  userRepo,
		apiURL:    apiURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ExternalAuth 通过外部API认证
func (h *ExternalAuthHandler) ExternalAuth(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		errors.RespondError(c, errors.New(errors.CodeUnauthorized, "缺少认证token"))
		return
	}

	// 调用外部API获取用户信息
	extUser, err := h.fetchExternalUser(token)
	if err != nil {
		logger.Warn("获取外部用户信息失败", zap.Error(err))
		errors.RespondError(c, errors.New(errors.CodeUnauthorized, "认证失败"))
		return
	}

	// 使用 ext_ 前缀避免与注册用户冲突
	extUsername := "ext_" + extUser.ID

	// 查找或创建用户（不改表结构）
	user, err := h.userRepo.GetByUsername(extUsername)
	if err != nil {
		user = &model.User{
			Username: extUsername,
			Nickname: extUser.Name,
			Avatar:   extUser.Avatar,
		}
		if err := h.userRepo.Create(user); err != nil {
			logger.Error("创建用户失败", zap.Error(err))
			errors.RespondError(c, errors.New(errors.CodeServerError, "创建用户失败"))
			return
		}
	} else {
		// 更新用户信息
		changed := false
		if extUser.Name != "" && extUser.Name != user.Nickname {
			user.Nickname = extUser.Name
			changed = true
		}
		if extUser.Avatar != "" && extUser.Avatar != user.Avatar {
			user.Avatar = extUser.Avatar
			changed = true
		}
		if changed {
			h.userRepo.UpdateProfile(user.ID, user.Nickname, user.Avatar)
		}
	}

	// 生成GoChat内部JWT
	jwtToken, err := util.GenerateToken(int(user.ID), user.Username, 48)
	if err != nil {
		errors.RespondError(c, errors.New(errors.CodeServerError, "生成token失败"))
		return
	}

	errors.RespondOK(c, gin.H{
		"token": jwtToken,
		"user": gin.H{
			"id":       user.ID,
			"name":     user.Nickname,
			"avatar":   user.Avatar,
			"username": user.Username,
		},
	})
}

// ExternalUser 外部系统用户信息
type ExternalUser struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

func (h *ExternalAuthHandler) fetchExternalUser(token string) (*ExternalUser, error) {
	req, err := http.NewRequest("GET", h.apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求外部API失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("外部API返回非200状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var extUser ExternalUser
	if err := json.Unmarshal(body, &extUser); err != nil {
		return nil, fmt.Errorf("解析外部用户信息失败: %w", err)
	}

	if extUser.ID == "" {
		return nil, fmt.Errorf("外部API未返回用户ID")
	}

	return &extUser, nil
}
```

- [ ] **Step 2: 添加 UserRepository 的 GetByExternalID 方法**

Modify `server/logic/registry/user_repository.go`:

```go
type UserRepository interface {
	Create(user *model.User) error
	GetByID(id int64) (*model.User, error)
	GetByUsername(username string) (*model.User, error)
	GetByExternalID(externalID string) (*model.User, error)  // ← 新增
	UpdateProfile(id int64, nickname, avatar string) error
	UpdateStatus(id int64, status int8) error
}
```

Add implementation:
```go
func (r *userRepository) GetByExternalID(externalID string) (*model.User, error) {
	var user model.User
	err := mysql.Client.Where("external_id = ?", externalID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build -tags=etcd ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/api/handler/auth_external.go server/logic/registry/user_repository.go
git commit -m "feat: 实现外部认证处理器(调外部API获取用户信息)"
```

---

### Task 4: 路由注册 + 配置注入

**Files:**
- Modify: `server/api/router.go`
- Modify: `server/main.go`

- [ ] **Step 1: 更新 router.go**

在 `NewRouter` 参数中添加 `authMode` 和 `authExternalAPI`，并注册新路由：

```go
func NewRouter(
	userRepo registry.UserRepository,
	friendRepo registry.FriendRepository,
	groupRepo registry.GroupRepository,
	groupMemberRepo registry.GroupMemberRepository,
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
	minioClient *minio.Client,
	authMode string,
	authExternalAPI string,
) *Router {
	// ... 现有代码 ...

	// 认证路由
	auth := api.Group("/auth")
	if authMode == "external" {
		authHandler := handler.NewExternalAuthHandler(userRepo, authExternalAPI)
		auth.POST("", authHandler.ExternalAuth)
	} else {
		auth.POST("/register", userHandler.Register)
		auth.POST("/login", userHandler.Login)
	}

	// ... 现有路由 ...

	return &Router{engine: engine}
}
```

- [ ] **Step 2: 更新 main.go**

```go
import "gochat/pkg/config"

func main() {
	// ... 加载配置后传给 NewRouter
	router := api.NewRouter(
		userRepo, friendRepo, groupRepo, groupMemberRepo,
		messageRepo, messageReadRepo, minioClient,
		cfg.Auth.Mode,          // ← 新增参数
		cfg.Auth.ExternalUserAPI, // ← 新增参数
	)
```

- [ ] **Step 3: 编译验证**

```bash
cd server && go build -tags=etcd ./...
```

- [ ] **Step 4: 提交**

```bash
git add server/api/router.go server/main.go
git commit -m "feat: 路由支持外部认证模式切换"
```

---

### Task 5: 外部认证模式下的前端适配

**Files:**
- Modify: `web/src/views/Login.vue`
- Modify: `web/src/stores/user.js`

- [ ] **Step 1: 更新 user store 支持外部认证**

```javascript
async function externalAuth(externalToken) {
  const res = await http.post('/auth', null, {
    headers: { Authorization: externalToken }
  })
  if (res.code !== 0) throw new Error(res.message)
  token.value = res.data.token
  user.value = res.data.user
  localStorage.setItem('token', res.data.token)
  localStorage.setItem('user', JSON.stringify(res.data.user))
  wsClient.connect(res.data.token)
  loadFriends()
}
```

- [ ] **Step 2: 提交**

```bash
git add web/src/stores/user.js web/src/views/Login.vue
git commit -m "feat: 前端支持外部认证模式"
```

---

### Task 6: 添加 /healthz 和路由兼容

**Files:**
- Modify: `server/api/router.go`

- [ ] **Step 1: 添加 healthz 端点**

```go
engine.GET("/healthz", func(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok", "time": time.Now().Unix()})
})
```

- [ ] **Step 2: 提交**

```bash
git add server/api/router.go
git commit -m "feat: 添加 /healthz 健康检查端点"
```

---

### Task 7: 前端组件化 — ChatWidget

**Files:**
- Create: `web/src/composables/useChat.js`
- Create: `web/src/components/ChatWidget.vue`

- [ ] **Step 1: 创建 useChat composable**

```javascript
import { ref, onMounted, onUnmounted } from 'vue'
import { wsClient } from '../api/ws'
import { chatStore } from '../stores/chat'

export function useChat(token, userId) {
  const connected = ref(false)
  const messages = ref([])
  const currentChat = ref(null)
  const unreadMap = ref({})

  function connect() {
    wsClient.connect(token)
  }

  function disconnect() {
    wsClient.disconnect()
  }

  function openChat(id, name) {
    currentChat.value = { id, name }
    chatStore.openChat(id, name)
  }

  function send(content) {
    chatStore.sendMessage(content)
  }

  onMounted(() => {
    wsClient.on('open', () => { connected.value = true })
    wsClient.on('close', () => { connected.value = false })
    connect()
  })

  onUnmounted(() => {
    disconnect()
  })

  return { connected, messages, currentChat, unreadMap, openChat, send }
}
```

- [ ] **Step 2: 创建 ChatWidget.vue**

```vue
<template>
  <div class="gochat-widget">
    <div class="gw-contacts">
      <div
        v-for="c in contacts"
        :key="c.id"
        :class="['gw-contact', { active: chat.currentChat?.id === c.id }]"
        @click="chat.openChat(c.id, c.name)"
      >
        <div class="gw-avatar">{{ (c.name || '?')[0] }}</div>
        <div class="gw-info">
          <div class="gw-name">{{ c.name }}</div>
          <div v-if="chat.unreadMap[c.id]" class="gw-badge">{{ chat.unreadMap[c.id] }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, watch } from 'vue'
import { useChat } from '../composables/useChat'

const props = defineProps({
  token: { type: String, required: true },
  userId: { type: [String, Number], required: true },
  contacts: { type: Array, default: () => [] }
})

const chat = useChat(props.token, props.userId)

watch(() => props.token, (t) => {
  if (t) chat.connect()
})
</script>

<style scoped>
.gochat-widget {
  font-family: 'Noto Sans SC', sans-serif;
  background: var(--bg, #0c0c0c);
  color: var(--text, #e8e8e8);
  height: 100%;
  display: flex;
}
.gw-contacts {
  width: 260px; border-right: 1px solid var(--border, #2a2a2a);
  overflow-y: auto; background: var(--bg-2, #141414);
}
.gw-contact {
  padding: 12px 16px; display: flex; align-items: center; gap: 10px;
  cursor: pointer; border-bottom: 1px solid var(--border, #2a2a2a);
  transition: background .15s;
}
.gw-contact:hover { background: var(--bg-3, #1c1c1c); }
.gw-contact.active { background: color-mix(in srgb, var(--accent, #e85d3a) 8%, var(--bg-4, #252525)); }
.gw-avatar {
  width: 36px; height: 36px; border-radius: 50%; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--accent, #e85d3a); font-weight: 600; font-size: 14px;
}
.gw-info { flex: 1; display: flex; align-items: center; }
.gw-name { flex: 1; font-size: 14px; font-weight: 500; }
.gw-badge {
  background: var(--accent, #e85d3a); color: #fff; font-size: 11px; font-weight: 600;
  padding: 2px 7px; border-radius: 10px; min-width: 20px; text-align: center;
}
</style>
```

- [ ] **Step 3: 提交**

```bash
git add web/src/composables/ web/src/components/
git commit -m "feat: 抽离 ChatWidget 可嵌入组件 + useChat composable"
```

---

### Task 8: 编译部署验证

- [ ] **Step 1: 全量编译**

```bash
cd server && go build -tags=etcd ./...
```

- [ ] **Step 2: 运行测试**

```bash
cd server && go test -tags=etcd ./test/... -count=1 -timeout 60s -run "TestHandlerRegistry|TestConnectServer"
```
