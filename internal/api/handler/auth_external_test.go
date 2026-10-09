package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gochat/internal/logic/model"
	bizerr "gochat/internal/pkg/errors"
	"gochat/internal/pkg/util"

	"github.com/gin-gonic/gin"
)

// mockUserRepo 模拟 UserRepository
type mockUserRepo struct {
	users map[string]*model.User
}

func (m *mockUserRepo) GetByUsername(username string) (*model.User, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return u, nil
}
func (m *mockUserRepo) GetByID(id int64) (*model.User, error)          { return nil, nil }
func (m *mockUserRepo) GetByExternalID(externalID string) (*model.User, error) { return nil, nil }
func (m *mockUserRepo) Create(user *model.User) error {
	if m.users == nil {
		m.users = make(map[string]*model.User)
	}
	m.users[user.Username] = user
	return nil
}
func (m *mockUserRepo) UpdateProfile(id int64, nickname, avatar string) error { return nil }
func (m *mockUserRepo) UpdateStatus(id int64, status int8) error              { return nil }
func (m *mockUserRepo) Delete(id int64) error                                 { return nil }

// mockExternalServer 模拟外部用户 API
func mockExternalServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{
			"id":   "42",
			"name": "张三",
		})
	}))
}

func TestExternalAuthFlow(t *testing.T) {
	extSrv := mockExternalServer()
	defer extSrv.Close()

	util.Init("test-secret")
	gin.SetMode(gin.TestMode)

	handler := NewExternalAuthHandler(&mockUserRepo{}, extSrv.URL)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/auth", nil)
	c.Request.Header.Set("Authorization", "Bearer mock-jwt")

	handler.ExternalAuth(c)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			User  struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if resp.Code != 0 {
		t.Fatalf("期望 code=0, 得到 %d: %s", resp.Code, w.Body.String())
	}
	if resp.Data.User.Name != "张三" {
		t.Errorf("期望 name=张三, 得到 %s", resp.Data.User.Name)
	}
	if resp.Data.User.Username != "ext_42" {
		t.Errorf("期望 username=ext_42, 得到 %s", resp.Data.User.Username)
	}
	if resp.Data.Token == "" {
		t.Error("期望非空 token")
	}
	t.Logf("外部认证通过: user_id=%d name=%s token=%s...",
		resp.Data.User.ID, resp.Data.User.Name, resp.Data.Token[:20])
}

func TestExternalAuthMissingToken(t *testing.T) {
	util.Init("test-secret")
	gin.SetMode(gin.TestMode)

	handler := NewExternalAuthHandler(&mockUserRepo{}, "http://localhost:9999")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/api/auth", nil)

	handler.ExternalAuth(c)

	var resp struct {
		Code int `json:"code"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != bizerr.CodeUnauthorized {
		t.Fatalf("期望 %d, 得到 %d", bizerr.CodeUnauthorized, resp.Code)
	}
	t.Log("缺失 token 时正确返回 401")
}
