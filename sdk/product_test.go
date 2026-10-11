package sdk_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gochat/sdk"
)

func TestSafeReadRetryNeverReplaysWrites(t *testing.T) {
	var reads, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && reads.Add(1) == 3 {
			io.WriteString(w, `{"code":0,"data":[]}`)
			return
		}
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		w.WriteHeader(503)
		io.WriteString(w, `{"code":503,"message":"unavailable"}`)
	}))
	defer server.Close()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("secret"), sdk.WithReadRetry(2, time.Millisecond))
	if _, err := client.ListFriends(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.AddFriend(context.Background(), 3, ""); err == nil {
		t.Fatal("write should fail")
	}
	if reads.Load() != 3 || writes.Load() != 1 {
		t.Fatalf("attempts reads=%d writes=%d", reads.Load(), writes.Load())
	}
}

func TestPerUserRefreshAndTransportTimeout(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer alice-fresh" {
			t.Errorf("auth=%q", r.Header.Get("Authorization"))
		}
		io.WriteString(w, `{"code":0,"data":[]}`)
	}))
	defer server.Close()
	base, _ := sdk.NewClient(server.URL, sdk.WithTimeout(time.Second), sdk.WithTransport(http.DefaultTransport), sdk.WithTokenRefresh(func(ctx context.Context, current string) (string, error) {
		refreshes.Add(1)
		return current + "-fresh", nil
	}))
	alice := base.WithToken("alice")
	if _, err := alice.ListFriends(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ListFriends(context.Background()); err == nil {
		t.Fatal("base must remain unauthenticated")
	}
	if refreshes.Load() != 1 {
		t.Fatalf("refreshes=%d", refreshes.Load())
	}
}

type captureLogger struct{ entries []string }

func (l *captureLogger) Log(_ context.Context, event string, fields map[string]any) {
	l.entries = append(l.entries, fmt.Sprint(event, fields))
}
func TestLoggerDoesNotExposeCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"code":401,"message":"token=private"}`)
	}))
	defer server.Close()
	logger := &captureLogger{}
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("private"), sdk.WithLogger(logger))
	client.ListFriends(context.Background())
	if len(logger.entries) == 0 {
		t.Fatal("no request log")
	}
	if strings.Contains(fmt.Sprint(logger.entries), "private") || strings.Contains(fmt.Sprint(logger.entries), server.URL) {
		t.Fatalf("unsafe logs: %v", logger.entries)
	}
}

func TestStreamingUploadProgressAndFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		kind, _ := reader.NextPart()
		data, _ := io.ReadAll(kind)
		if string(data) != "file" {
			t.Error("missing type")
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		if part.FileName() != `a";b.txt` {
			t.Errorf("filename=%q", part.FileName())
		}
		n, err := io.Copy(io.Discard, part)
		if err != nil || n != 1048576 {
			t.Errorf("size=%d error=%v", n, err)
		}
		io.WriteString(w, `{"code":0,"data":{"name":"ok","size":1048576}}`)
	}))
	defer server.Close()
	var last int64
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	meta, err := client.Upload(context.Background(), sdk.UploadRequest{Type: "file", Filename: `a";b.txt`, File: bytes.NewReader(make([]byte, 1048576)), Size: 1048576, Progress: func(sent, total int64) {
		if total != 1048576 || sent < last {
			t.Error("invalid progress")
		}
		last = sent
	}})
	if err != nil {
		t.Fatal(err)
	}
	if last != 1048576 || meta.Size != 1048576 {
		t.Fatalf("last=%d meta=%+v", last, meta)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCustomTransportHonorsTimeout(t *testing.T) {
	client, _ := sdk.NewClient("http://unreachable.invalid", sdk.WithToken("token"), sdk.WithTimeout(20*time.Millisecond), sdk.WithTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })))
	started := time.Now()
	_, err := client.ListFriends(context.Background())
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(started) > time.Second {
		t.Fatal("timeout not honored")
	}
}

// A producer cannot yield its remaining bytes until the server observes the
// first file bytes. Whole-file buffering would deadlock this public upload.
type gatedUploadReader struct {
	remaining int
	first     bool
	observed  <-chan struct{}
	ctx       context.Context
}

func (r *gatedUploadReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if r.first {
		select {
		case <-r.observed:
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	r.first = true
	return n, nil
}
func TestUploadStartsSendingBeforeReadingWholeFile(t *testing.T) {
	observed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		part, _ := reader.NextPart()
		io.Copy(io.Discard, part)
		part, err = reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		buffer := make([]byte, 1)
		if _, err = part.Read(buffer); err != nil {
			t.Error(err)
			return
		}
		close(observed)
		n, err := io.Copy(io.Discard, part)
		if err != nil || n != 2*1024*1024-1 {
			t.Errorf("n=%d err=%v", n, err)
		}
		io.WriteString(w, `{"code":0,"data":{"size":2097152}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, _ := sdk.NewClient(server.URL, sdk.WithToken("token"))
	_, err := client.Upload(ctx, sdk.UploadRequest{Filename: "large.bin", Type: "file", File: &gatedUploadReader{remaining: 2 * 1024 * 1024, observed: observed, ctx: ctx}})
	if err != nil {
		t.Fatal(err)
	}
}
