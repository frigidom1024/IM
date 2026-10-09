package minio

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"gochat/internal/pkg/config"
)

// FileMeta 文件元数据
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

// Client MinIO 客户端
type Client struct {
	client *minio.Client
	bucket string
}

// NewClient 创建 MinIO 客户端
func NewClient(cfg *config.MinIOConfig) (*Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO 客户端失败: %w", err)
	}

	ctx := context.Background()
	if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
		exists, errBucket := client.BucketExists(ctx, cfg.Bucket)
		if errBucket != nil || !exists {
			return nil, fmt.Errorf("创建桶失败: %w", err)
		}
	}

	return &Client{client: client, bucket: cfg.Bucket}, nil
}

// Upload 上传文件，返回 FileMeta
func (c *Client) Upload(ctx context.Context, objectPath string, reader io.Reader, size int64, contentType string) (*FileMeta, error) {
	_, err := c.client.PutObject(ctx, c.bucket, objectPath, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %w", err)
	}

	return &FileMeta{
		URL:  objectPath,
		Name: path.Base(objectPath),
		Size: size,
		MIME: contentType,
	}, nil
}

// PresignedURL 生成临时访问链接
func (c *Client) PresignedURL(ctx context.Context, objectPath string, expires time.Duration) (string, error) {
	url, err := c.client.PresignedGetObject(ctx, c.bucket, objectPath, expires, nil)
	if err != nil {
		return "", fmt.Errorf("生成签名 URL 失败: %w", err)
	}
	return url.String(), nil
}

// Delete 删除文件
func (c *Client) Delete(ctx context.Context, objectPath string) error {
	err := c.client.RemoveObject(ctx, c.bucket, objectPath, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("删除文件失败: %w", err)
	}
	return nil
}
