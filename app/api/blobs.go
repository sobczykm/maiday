package main

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Blobs keeps file contents in an S3-compatible bucket; metadata lives in the assets table.
type Blobs struct {
	client *minio.Client
	bucket string
}

func newBlobs(cfg Config) (*Blobs, error) {
	if !cfg.FileStorageEnabled() {
		return nil, nil
	}
	client, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKeyID, cfg.S3SecretAccessKey, ""),
		Secure: true,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, err
	}
	return &Blobs{client: client, bucket: cfg.S3Bucket}, nil
}

func objectKey(a Asset) string {
	return a.UserID + "/" + a.ID
}

func (b *Blobs) Put(ctx context.Context, a Asset, body io.Reader) error {
	_, err := b.client.PutObject(ctx, b.bucket, objectKey(a), body, a.SizeBytes, minio.PutObjectOptions{
		ContentType:      a.ContentType,
		DisableMultipart: true,
	})
	return err
}

func (b *Blobs) Get(ctx context.Context, a Asset) (io.ReadCloser, error) {
	obj, err := b.client.GetObject(ctx, b.bucket, objectKey(a), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, err
	}
	return obj, nil
}

func (b *Blobs) Delete(ctx context.Context, a Asset) error {
	return b.client.RemoveObject(ctx, b.bucket, objectKey(a), minio.RemoveObjectOptions{})
}
