package minio

import (
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"io"
)

// Open returns a seekable object for authorized streaming and range requests.
func (c *Client) Open(ctx context.Context, objectPath string) (io.ReadSeekCloser, error) {
	object, err := c.client.GetObject(ctx, c.bucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err = object.Stat(); err != nil {
		object.Close()
		return nil, err
	}
	return object, nil
}

// SetUploadLifecycle expires staging objects and abandoned multipart uploads.
// Permanent message attachments are expired by database-aware maintenance.
func (c *Client) SetUploadLifecycle(ctx context.Context, days int) error {
	return c.client.SetBucketLifecycle(ctx, c.bucket, &lifecycle.Configuration{Rules: []lifecycle.Rule{{
		ID: "gochat-staging", Status: "Enabled", RuleFilter: lifecycle.Filter{Prefix: "staging/"},
		Expiration:                     lifecycle.Expiration{Days: lifecycle.ExpirationDays(days)},
		AbortIncompleteMultipartUpload: lifecycle.AbortIncompleteMultipartUpload{DaysAfterInitiation: lifecycle.ExpirationDays(days)},
	}}})
}
