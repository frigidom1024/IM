package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrMissingToken indicates that an authenticated operation was attempted
	// without a GoChat token.
	ErrMissingToken = errors.New("gochat sdk: missing token")
)

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient supplies the HTTP client used by Client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithToken sets the GoChat JWT used by authenticated requests.
func WithToken(token string) Option {
	return func(c *Client) {
		c.token = NormalizeToken(token)
	}
}

// WithWebSocketURL configures the WebSocket endpoint. This is useful when
// HTTP and WebSocket traffic use different ports, as in a direct GoChat
// deployment. A reverse-proxied deployment can omit this option.
func WithWebSocketURL(endpoint string) Option {
	return func(c *Client) {
		c.websocketURL = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	}
}

// Client is a typed client for the GoChat HTTP and WebSocket APIs.
type Client struct {
	baseURL      string
	httpClient   *http.Client
	token        string
	websocketURL string
}

// NewClient creates a GoChat SDK client.
//
// baseURL should point to the GoChat HTTP server, for example
// "http://localhost:8080". The /api prefix is added by the SDK.
func NewClient(baseURL string, options ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("gochat sdk: base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("gochat sdk: invalid base URL %q", baseURL)
	}

	client := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}
	return client, nil
}

// WithToken returns a copy of the client using token for authenticated calls.
// The original client is not mutated, which makes it safe to keep one client
// and derive per-user clients in a backend application.
func (c *Client) WithToken(token string) *Client {
	clone := *c
	clone.token = NormalizeToken(token)
	return &clone
}

// ExternalAuth exchanges an external application's token for a GoChat JWT.
func (c *Client) ExternalAuth(ctx context.Context, externalToken string) (*AuthResponse, error) {
	var response AuthResponse
	err := c.do(ctx, http.MethodPost, "/api/auth", externalToken, nil, &response)
	return &response, err
}

// Register creates a builtin GoChat user.
func (c *Client) Register(ctx context.Context, request RegisterRequest) error {
	return c.do(ctx, http.MethodPost, "/api/auth/register", "", request, nil)
}

// Login logs a builtin GoChat user in.
func (c *Client) Login(ctx context.Context, request LoginRequest) (*AuthResponse, error) {
	var response AuthResponse
	err := c.do(ctx, http.MethodPost, "/api/auth/login", "", request, &response)
	return &response, err
}

// GetUserInfo gets a user by GoChat user ID.
func (c *Client) GetUserInfo(ctx context.Context, userID int64) (*User, error) {
	var response User
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/user/info/"+strconv.FormatInt(userID, 10), nil, &response)
	return &response, err
}

// UpdateProfile updates the current user's profile.
func (c *Client) UpdateProfile(ctx context.Context, request UpdateProfileRequest) error {
	return c.doAuthenticated(ctx, http.MethodPut, "/api/user/profile", request, nil)
}

// UpdateStatus updates the current user's status.
func (c *Client) UpdateStatus(ctx context.Context, status int8) error {
	return c.doAuthenticated(ctx, http.MethodPut, "/api/user/status", UpdateStatusRequest{Status: status}, nil)
}

// ListFriends gets the current user's friends.
func (c *Client) ListFriends(ctx context.Context) ([]Friend, error) {
	var response []Friend
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/friend/list", nil, &response)
	return response, err
}

// AddFriend creates a friend relation for the current user.
func (c *Client) AddFriend(ctx context.Context, friendID int64, remark string) error {
	return c.doAuthenticated(ctx, http.MethodPost, "/api/friend/add", AddFriendRequest{
		FriendID: friendID,
		Remark:   remark,
	}, nil)
}

// RemoveFriend removes a friend relation for the current user.
func (c *Client) RemoveFriend(ctx context.Context, friendID int64) error {
	return c.doAuthenticated(ctx, http.MethodDelete, "/api/friend/"+strconv.FormatInt(friendID, 10), nil, nil)
}

// ListGroups gets groups visible to the current user.
func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	var response []Group
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/group/list", nil, &response)
	return response, err
}

// CreateGroup creates a group and adds the current user as owner.
func (c *Client) CreateGroup(ctx context.Context, request CreateGroupRequest) (*Group, error) {
	var response Group
	err := c.doAuthenticated(ctx, http.MethodPost, "/api/group/create", request, &response)
	return &response, err
}

// JoinGroup adds the current user to a group.
func (c *Client) JoinGroup(ctx context.Context, groupID int64) error {
	return c.doAuthenticated(ctx, http.MethodPost, "/api/group/"+strconv.FormatInt(groupID, 10)+"/join", nil, nil)
}

// LeaveGroup removes the current user from a group.
func (c *Client) LeaveGroup(ctx context.Context, groupID int64) error {
	return c.doAuthenticated(ctx, http.MethodDelete, "/api/group/"+strconv.FormatInt(groupID, 10)+"/leave", nil, nil)
}

// ListGroupMembers gets the members of a group.
func (c *Client) ListGroupMembers(ctx context.Context, groupID int64) ([]GroupMember, error) {
	var response []GroupMember
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/group/"+strconv.FormatInt(groupID, 10)+"/members", nil, &response)
	return response, err
}

// RemoveGroupMember removes a member from a group.
func (c *Client) RemoveGroupMember(ctx context.Context, groupID, userID int64) error {
	path := "/api/group/" + strconv.FormatInt(groupID, 10) + "/members/" + strconv.FormatInt(userID, 10)
	return c.doAuthenticated(ctx, http.MethodDelete, path, nil, nil)
}

// ListPrivateMessages gets recent private messages with another user.
func (c *Client) ListPrivateMessages(ctx context.Context, userID int64) ([]Message, error) {
	return c.ListPrivateMessagesPage(ctx, userID, 50, 0)
}

// ListPrivateMessagesPage gets a page of private messages.
func (c *Client) ListPrivateMessagesPage(ctx context.Context, userID int64, limit int, offset int64) ([]Message, error) {
	var response []Message
	path := fmt.Sprintf("/api/message/private/%d?limit=%d&offset=%d", userID, limit, offset)
	err := c.doAuthenticated(ctx, http.MethodGet, path, nil, &response)
	return response, err
}

// ListGroupMessages gets recent messages in a group.
func (c *Client) ListGroupMessages(ctx context.Context, groupID int64) ([]Message, error) {
	return c.ListGroupMessagesPage(ctx, groupID, 50, 0)
}

// ListGroupMessagesPage gets a page of group messages.
func (c *Client) ListGroupMessagesPage(ctx context.Context, groupID int64, limit int, offset int64) ([]Message, error) {
	var response []Message
	path := fmt.Sprintf("/api/message/group/%d?limit=%d&offset=%d", groupID, limit, offset)
	err := c.doAuthenticated(ctx, http.MethodGet, path, nil, &response)
	return response, err
}

// MarkRead clears private-message unread count from a sender.
func (c *Client) MarkRead(ctx context.Context, fromID int64) error {
	return c.MarkReadConversation(ctx, fromID, 0)
}

// MarkReadConversation clears unread count for one private or group scope.
func (c *Client) MarkReadConversation(ctx context.Context, fromID, groupID int64) error {
	return c.doAuthenticated(ctx, http.MethodPost, "/api/message/read", MarkReadRequest{FromID: fromID, GroupID: groupID}, nil)
}

// GetUnreadCount gets the current user's total unread count.
func (c *Client) GetUnreadCount(ctx context.Context) (int64, error) {
	response, err := c.GetUnreadSummary(ctx)
	return response.Count, err
}

// GetUnreadSummary gets the total unread count and one summary per private or
// group scope.
func (c *Client) GetUnreadSummary(ctx context.Context) (*UnreadCountResponse, error) {
	var response UnreadCountResponse
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/message/unread", nil, &response)
	return &response, err
}

// Upload uploads a file or media object through the GoChat API.
func (c *Client) Upload(ctx context.Context, request UploadRequest) (*FileMeta, error) {
	if request.File == nil {
		return nil, errors.New("gochat sdk: upload file is required")
	}
	if request.Filename == "" {
		return nil, errors.New("gochat sdk: upload filename is required")
	}
	if request.Type == "" {
		return nil, errors.New("gochat sdk: upload type is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", request.Type); err != nil {
		return nil, fmt.Errorf("gochat sdk: write upload type: %w", err)
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, request.Filename)}
	if request.ContentType != "" {
		partHeader["Content-Type"] = []string{request.ContentType}
	}
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, fmt.Errorf("gochat sdk: create upload part: %w", err)
	}
	if _, err := io.Copy(part, request.File); err != nil {
		return nil, fmt.Errorf("gochat sdk: read upload file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gochat sdk: close upload body: %w", err)
	}

	var response FileMeta
	err = c.doAuthenticatedWithContentType(ctx, http.MethodPost, "/api/upload", &body, writer.FormDataContentType(), &response)
	return &response, err
}

func (c *Client) doAuthenticated(ctx context.Context, method, path string, payload, output any) error {
	if c.token == "" {
		return ErrMissingToken
	}
	return c.do(ctx, method, path, c.token, payload, output)
}

func (c *Client) do(ctx context.Context, method, path, token string, payload, output any) error {
	return c.doWithToken(ctx, method, path, token, payload, "", output)
}

func (c *Client) doAuthenticatedWithContentType(ctx context.Context, method, path string, payload io.Reader, contentType string, output any) error {
	if c.token == "" {
		return ErrMissingToken
	}
	return c.doWithReader(ctx, method, path, c.token, payload, contentType, output)
}

func (c *Client) doWithToken(ctx context.Context, method, path, token string, payload any, contentType string, output any) error {
	var reader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("gochat sdk: encode request: %w", err)
		}
		reader = bytes.NewReader(data)
		contentType = "application/json"
	}
	return c.doWithReader(ctx, method, path, token, reader, contentType, output)
}

func (c *Client) doWithReader(ctx context.Context, method, path, token string, reader io.Reader, contentType string, output any) error {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("gochat sdk: create request: %w", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+NormalizeToken(token))
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("gochat sdk: request %s %s: %w", method, path, err)
	}
	defer response.Body.Close()

	var envelope responseEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			return fmt.Errorf("gochat sdk: decode response: %w", err)
		}
		return &APIError{HTTPStatus: response.StatusCode, Message: response.Status}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || envelope.Code != 0 {
		return &APIError{HTTPStatus: response.StatusCode, Code: envelope.Code, Message: envelope.Message}
	}
	if output == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("gochat sdk: decode data: %w", err)
	}
	return nil
}

func NormalizeToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) >= len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(token[len("Bearer "):])
	}
	return token
}
