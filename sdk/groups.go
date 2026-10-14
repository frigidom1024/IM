package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// GroupMemberPatch updates only supplied member settings. Role 0/1 represents
// member/admin; ownership transfer uses TransferGroupOwner instead.
type GroupMemberPatch struct {
	Nickname   *string    `json:"nickname,omitempty"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
	Unmute     bool       `json:"unmute,omitempty"`
	Role       *int8      `json:"role,omitempty"`
}

func (c *Client) SearchGroups(ctx context.Context, query string, limit int, offset int64) ([]Group, error) {
	var result []Group
	err := c.doAuthenticated(ctx, http.MethodGet, fmt.Sprintf("/api/group/search?q=%s&limit=%d&offset=%d", url.QueryEscape(query), limit, offset), nil, &result)
	return result, err
}
func (c *Client) InviteGroupMember(ctx context.Context, groupID, userID int64) error {
	return c.doAuthenticated(ctx, http.MethodPost, fmt.Sprintf("/api/group/%d/invite", groupID), map[string]int64{"user_id": userID}, nil)
}
func (c *Client) TransferGroupOwner(ctx context.Context, groupID, userID int64) error {
	return c.doAuthenticated(ctx, http.MethodPost, fmt.Sprintf("/api/group/%d/transfer", groupID), map[string]int64{"user_id": userID}, nil)
}
func (c *Client) UpdateGroupAnnouncement(ctx context.Context, groupID int64, announcement string) error {
	return c.doAuthenticated(ctx, http.MethodPut, fmt.Sprintf("/api/group/%d/announcement", groupID), map[string]string{"announcement": announcement}, nil)
}
func (c *Client) UpdateGroupMember(ctx context.Context, groupID, userID int64, patch GroupMemberPatch) error {
	return c.doAuthenticated(ctx, http.MethodPatch, fmt.Sprintf("/api/group/%d/members/%d", groupID, userID), patch, nil)
}
func (c *Client) DissolveGroup(ctx context.Context, groupID int64) error {
	return c.doAuthenticated(ctx, http.MethodDelete, fmt.Sprintf("/api/group/%d", groupID), nil, nil)
}
func (c *Client) ListGroupMembersPage(ctx context.Context, groupID int64, limit int, offset int64) ([]GroupMember, error) {
	var result []GroupMember
	err := c.doAuthenticated(ctx, http.MethodGet, fmt.Sprintf("/api/group/%d/members?limit=%d&offset=%d", groupID, limit, offset), nil, &result)
	return result, err
}
