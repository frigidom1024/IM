package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// CreateUploadSessionRequest declares the immutable asset metadata.
type CreateUploadSessionRequest struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	MIME         string `json:"mime"`
	Size         int64  `json:"size"`
	GroupID      int64  `json:"group_id,omitempty"`
	ExpiresHours int    `json:"expires_hours,omitempty"`
}
type UploadSession struct {
	ID        string    `json:"id"`
	GroupID   int64     `json:"group_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	MIME      string    `json:"mime"`
	Size      int64     `json:"size"`
	Offset    int64     `json:"offset"`
	AssetID   string    `json:"asset_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func uploadSessionPath(id string) string { return "/api/upload/session/" + url.PathEscape(id) }
func (c *Client) CreateUploadSession(ctx context.Context, request CreateUploadSessionRequest) (*UploadSession, error) {
	var result UploadSession
	err := c.doAuthenticated(ctx, http.MethodPost, "/api/upload/session", request, &result)
	return &result, err
}
func (c *Client) GetUploadSession(ctx context.Context, id string) (*UploadSession, error) {
	var result UploadSession
	err := c.doAuthenticated(ctx, http.MethodGet, uploadSessionPath(id), nil, &result)
	return &result, err
}

// AppendUploadChunk sends a single chunk, at most 8 MiB, without replaying it.
func (c *Client) AppendUploadChunk(ctx context.Context, id string, offset int64, chunk []byte) (*UploadSession, error) {
	if offset < 0 || len(chunk) == 0 || len(chunk) > 8<<20 {
		return nil, errors.New("gochat sdk: invalid upload offset or chunk size")
	}
	token, err := c.authToken(ctx)
	if err != nil {
		return nil, err
	}
	var result UploadSession
	err = c.doWithReaderHeaders(ctx, http.MethodPatch, uploadSessionPath(id), token, bytes.NewReader(chunk), "application/octet-stream", http.Header{"Upload-Offset": []string{strconv.FormatInt(offset, 10)}}, &result)
	return &result, err
}
func (c *Client) CompleteUploadSession(ctx context.Context, id string) (*FileMeta, error) {
	var result FileMeta
	err := c.doAuthenticated(ctx, http.MethodPost, uploadSessionPath(id)+"/complete", nil, &result)
	return &result, err
}
func (c *Client) AbortUploadSession(ctx context.Context, id string) error {
	return c.doAuthenticated(ctx, http.MethodDelete, uploadSessionPath(id), nil, nil)
}
func (c *Client) ListGroupFiles(ctx context.Context, groupID int64, limit int, offset int64) ([]FileMeta, error) {
	var result []FileMeta
	err := c.doAuthenticated(ctx, http.MethodGet, fmt.Sprintf("/api/group/%d/files?limit=%d&offset=%d", groupID, limit, offset), nil, &result)
	return result, err
}

// UploadFileRequest sends a seekable input in bounded 8 MiB chunks. SessionID
// resumes a known session from the server's persisted offset. Progress reports
// acknowledged bytes and is invoked synchronously; Size must match the input.
type UploadFileRequest struct {
	File             io.ReadSeeker
	SessionID        string
	Name, Type, MIME string
	Size             int64
	GroupID          int64
	ExpiresHours     int
	Progress         func(acknowledged, total int64)
}

// UploadSessionError preserves the session ID for resuming after a failed write.
// A failed chunk is never retried automatically; query the session on resume.
type UploadSessionError struct {
	SessionID string
	Err       error
}

func (e *UploadSessionError) Error() string {
	return fmt.Sprintf("gochat sdk: upload session %s: %v", e.SessionID, e.Err)
}
func (e *UploadSessionError) Unwrap() error { return e.Err }
func (c *Client) UploadFile(ctx context.Context, request UploadFileRequest) (*FileMeta, error) {
	if request.File == nil || request.Size <= 0 {
		return nil, errors.New("gochat sdk: seekable file and positive size required")
	}
	actual, err := request.File.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if actual != request.Size {
		return nil, errors.New("gochat sdk: declared upload size differs from input")
	}
	var session *UploadSession
	if request.SessionID != "" {
		session, err = c.GetUploadSession(ctx, request.SessionID)
	} else {
		session, err = c.CreateUploadSession(ctx, CreateUploadSessionRequest{Name: request.Name, Type: request.Type, MIME: request.MIME, Size: request.Size, GroupID: request.GroupID, ExpiresHours: request.ExpiresHours})
	}
	if err != nil {
		return nil, err
	}
	failed := func(err error) (*FileMeta, error) { return nil, &UploadSessionError{SessionID: session.ID, Err: err} }
	if session.ID == "" || session.Size != request.Size || session.Offset < 0 || session.Offset > session.Size {
		return failed(errors.New("gochat sdk: inconsistent upload session"))
	}
	if _, err = request.File.Seek(session.Offset, io.SeekStart); err != nil {
		return failed(err)
	}
	if request.Progress != nil {
		request.Progress(session.Offset, session.Size)
	}
	buffer := make([]byte, int(min64(session.Size-session.Offset, 8<<20)))
	for session.Offset < session.Size {
		if err = ctx.Err(); err != nil {
			return failed(err)
		}
		chunk := buffer[:int(min64(session.Size-session.Offset, int64(len(buffer))))]
		if _, err = io.ReadFull(request.File, chunk); err != nil {
			return failed(err)
		}
		next, err := c.AppendUploadChunk(ctx, session.ID, session.Offset, chunk)
		if err != nil {
			return failed(err)
		}
		if next.ID != session.ID || next.Size != session.Size || next.Offset != session.Offset+int64(len(chunk)) {
			return failed(errors.New("gochat sdk: invalid acknowledged offset"))
		}
		session = next
		if request.Progress != nil {
			request.Progress(session.Offset, session.Size)
		}
	}
	meta, err := c.CompleteUploadSession(ctx, session.ID)
	if err != nil {
		return failed(err)
	}
	return meta, nil
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// DownloadFile returns an authenticated streaming response. The caller must
// close Body. HTTP error envelopes are converted to APIError before returning.
func (c *Client) DownloadFile(ctx context.Context, fileID string) (*http.Response, error) {
	return c.download(ctx, fileID, "download")
}
func (c *Client) DownloadThumbnail(ctx context.Context, fileID string) (*http.Response, error) {
	return c.download(ctx, fileID, "thumbnail")
}
func (c *Client) download(ctx context.Context, fileID, kind string) (*http.Response, error) {
	token, err := c.authToken(ctx)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/file/"+url.PathEscape(fileID)+"/"+kind, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	started := time.Now()
	response, err := c.httpClient.Do(request)
	if c.logger != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		c.logger.Log(ctx, "http_request", map[string]any{"method": http.MethodGet, "status": status, "attempt": 1, "duration_ms": time.Since(started).Milliseconds(), "transport_error": err != nil})
	}
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		var envelope responseEnvelope
		if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope) != nil {
			return nil, &APIError{HTTPStatus: response.StatusCode, Message: response.Status}
		}
		return nil, &APIError{HTTPStatus: response.StatusCode, Code: envelope.Code, Message: envelope.Message}
	}
	return response, nil
}
