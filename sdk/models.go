package sdk

import (
	"io"
	"time"
)

// AuthResponse is returned by external authentication and builtin login.
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// User represents a GoChat user. External authentication responses use Name;
// builtin user responses use Nickname. Both fields are kept for compatibility.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Status   int8   `json:"status"`
}

// RegisterRequest creates a builtin user.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// LoginRequest logs in a builtin user.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateProfileRequest updates the current user's profile.
type UpdateProfileRequest struct {
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// UpdateStatusRequest updates the current user's status.
type UpdateStatusRequest struct {
	Status int8 `json:"status"`
}

// Friend is a directed friend relation.
type Friend struct {
	AddedByPeer     bool       `json:"added_by_peer"`
	RecipientSeenAt *time.Time `json:"recipient_seen_at"`
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	FriendID        int64      `json:"friend_id"`
	Remark          string     `json:"remark"`
	CreatedAt       time.Time  `json:"created_at"`
}

// AddFriendRequest creates a friend relation.
type AddFriendRequest struct {
	FriendID int64  `json:"friend_id"`
	Remark   string `json:"remark"`
}

// Group represents a group and the viewer's membership, independently of a conversation.
type Group struct {
	MemberCount  int64     `json:"member_count"`
	PendingCount int64     `json:"pending_count"`
	Role         int8      `json:"role"`
	Announcement string    `json:"announcement"`
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Avatar       string    `json:"avatar"`
	OwnerID      int64     `json:"owner_id"`
	MaxMembers   int       `json:"max_members"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateGroupRequest creates a group.
type CreateGroupRequest struct {
	Name       string `json:"name"`
	Avatar     string `json:"avatar,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
}

// GroupMember represents membership in a group.
type GroupMember struct {
	Nickname   string     `json:"nickname"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
	ID         int64      `json:"id"`
	GroupID    int64      `json:"group_id"`
	UserID     int64      `json:"user_id"`
	Role       int8       `json:"role"`
	JoinedAt   time.Time  `json:"joined_at"`
}

// Message is a message returned by the history API.
type Message struct {
	ID           int64     `json:"id"`
	FromID       int64     `json:"from_id"`
	ToID         int64     `json:"to_id"`
	GroupID      int64     `json:"group_id"`
	MsgType      string    `json:"msg_type"`
	Content      string    `json:"content"`
	IsRevoked    bool      `json:"is_revoked"`
	CreatedAt    time.Time `json:"created_at"`
	SenderName   string    `json:"sender_name"`
	SenderAvatar string    `json:"sender_avatar"`
	SenderID     int64     `json:"sender_id"`
}

// UnreadCountResponse is returned by the unread count endpoint.
type UnreadCountResponse struct {
	Count      int64           `json:"count"`
	UnreadList []UnreadSummary `json:"unread_list"`
}

// UnreadSummary is one private or group unread aggregate.
type UnreadSummary struct {
	ID              string     `json:"id"`
	FromID          int64      `json:"from_id"`
	GroupID         int64      `json:"group_id"`
	Count           int64      `json:"count,omitempty"`
	UnreadCount     int64      `json:"unread_count"`
	LatestMessageID int64      `json:"latest_message_id,string,omitempty"`
	LatestSenderID  int64      `json:"latest_sender_id,omitempty"`
	LatestType      string     `json:"latest_message_type"`
	LatestPreview   string     `json:"latest_preview"`
	LatestAt        *time.Time `json:"latest_message_at,omitempty"`
}

// UploadRequest describes a file upload.
type UploadRequest struct {
	// File is read incrementally. The caller owns it and should make blocking
	// readers cancellation-aware. Progress executes on the upload goroutine.
	File io.Reader
	// Size is the total file size for progress (zero means unknown).
	Size int64
	// Progress reports bytes read from File, not bytes acknowledged by storage.
	Progress    func(sent, total int64)
	Filename    string
	ContentType string
	Type        string
}

// FileMeta describes an uploaded file.
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

// ExternalUser is the response contract that an external application exposes
// to GoChat at /api/user/info.
type ExternalUser struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}
