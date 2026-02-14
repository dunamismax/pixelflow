package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint string
	Access   string
	Secret   string
	Bucket   string
	UseSSL   bool
}

type Client struct {
	minio  *minio.Client
	bucket string
}

func NewClient(cfg Config) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.Access, cfg.Secret, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	return &Client{
		minio:  mc,
		bucket: cfg.Bucket,
	}, nil
}

func (c *Client) Bucket() string {
	return c.bucket
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.minio.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check bucket existence: %w", err)
	}
	if exists {
		return nil
	}

	if err := c.minio.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
		exists, checkErr := c.minio.BucketExists(ctx, c.bucket)
		if checkErr == nil && exists {
			return nil
		}
		return fmt.Errorf("create bucket %s: %w", c.bucket, err)
	}

	return nil
}

func (c *Client) PresignedPutURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	u, err := c.minio.PresignedPutObject(ctx, c.bucket, objectKey, expiry)
	if err != nil {
		return "", fmt.Errorf("presign put object: %w", err)
	}
	return u.String(), nil
}

func (c *Client) ObjectExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := c.minio.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}

	resp := minio.ToErrorResponse(err)
	if resp.Code == "NoSuchKey" || resp.Code == "NoSuchObject" {
		return false, nil
	}
	return false, fmt.Errorf("stat object %s: %w", objectKey, err)
}

func (c *Client) ReadObject(ctx context.Context, objectKey string) ([]byte, error) {
	obj, err := c.minio.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", objectKey, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", objectKey, err)
	}
	return data, nil
}

func (c *Client) WriteObject(ctx context.Context, objectKey string, data []byte, contentType string) error {
	reader := bytes.NewReader(data)
	_, err := c.minio.PutObject(
		ctx,
		c.bucket,
		objectKey,
		reader,
		int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return fmt.Errorf("put object %s: %w", objectKey, err)
	}
	return nil
}
