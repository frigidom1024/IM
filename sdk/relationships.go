package sdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var ErrGroupApprovalPending = errors.New("gochat sdk: group application pending owner approval")

type FriendNotice struct {
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar"`
	Unread    bool      `json:"unread"`
	AddedByMe bool      `json:"added_by_me"`
	CreatedAt time.Time `json:"created_at"`
}
type GroupApplication struct {
	ID            int64      `json:"id"`
	GroupID       int64      `json:"group_id"`
	UserID        int64      `json:"user_id"`
	GroupName     string     `json:"group_name"`
	ApplicantName string     `json:"applicant_name"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	HandledBy     int64      `json:"handled_by"`
	CreatedAt     time.Time  `json:"created_at"`
	HandledAt     *time.Time `json:"handled_at"`
}

func (c *Client) ListFriendNotices(ctx context.Context) ([]FriendNotice, error) {
	var result []FriendNotice
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/friend/incoming", nil, &result)
	return result, err
}
func (c *Client) ReadFriendNotice(ctx context.Context, userID int64) error {
	return c.doAuthenticated(ctx, http.MethodPost, fmt.Sprintf("/api/friend/%d/notice/read", userID), nil, nil)
}

// ApplyToGroup submits a request; pending never means membership was granted.
func (c *Client) ApplyToGroup(ctx context.Context, groupID int64) (*GroupApplication, error) {
	var result GroupApplication
	err := c.doAuthenticated(ctx, http.MethodPost, fmt.Sprintf("/api/group/%d/join", groupID), nil, &result)
	return &result, err
}
func (c *Client) ListMyGroupApplications(ctx context.Context, limit int, offset int64) ([]GroupApplication, error) {
	var result []GroupApplication
	err := c.doAuthenticated(ctx, http.MethodGet, fmt.Sprintf("/api/group/applications/mine?limit=%d&offset=%d", limit, offset), nil, &result)
	return result, err
}
func (c *Client) ListGroupApplications(ctx context.Context, groupID int64, limit int, offset int64) ([]GroupApplication, error) {
	var result []GroupApplication
	err := c.doAuthenticated(ctx, http.MethodGet, fmt.Sprintf("/api/group/%d/applications?limit=%d&offset=%d", groupID, limit, offset), nil, &result)
	return result, err
}
func (c *Client) ReviewGroupApplication(ctx context.Context, groupID, applicationID int64, decision, reason string) (*GroupApplication, error) {
	var result GroupApplication
	err := c.doAuthenticated(ctx, http.MethodPost, fmt.Sprintf("/api/group/%d/applications/%d/review", groupID, applicationID), map[string]string{"decision": decision, "reason": reason}, &result)
	return &result, err
}
