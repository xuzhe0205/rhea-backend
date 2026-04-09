package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

const (
	MaxImageSize  = 10 << 20 // 10 MB per image
	MaxTotalSize  = 20 << 20 // 20 MB total per request
	MaxImageCount = 5

	PresignTTL = 1 * time.Hour
)

var allowedMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

type R2Client struct {
	client *s3.Client
	bucket string
}

func NewR2Client(accountID, accessKeyID, secretAccessKey, bucket string) *R2Client {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)

	client := s3.NewFromConfig(aws.Config{
		Region: "auto",
		Credentials: credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"",
		),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return &R2Client{client: client, bucket: bucket}
}

// PutImage uploads image bytes to R2 and returns the object key.
// userID is used to namespace keys so users can't enumerate each other's uploads.
func (r *R2Client) PutImage(ctx context.Context, userID string, filename string, contentType string, body io.Reader) (string, error) {
	if !allowedMIMEs[contentType] {
		// fall back to extension-based detection
		ext := filepath.Ext(filename)
		contentType = mime.TypeByExtension(ext)
		if !allowedMIMEs[contentType] {
			return "", fmt.Errorf("unsupported image type: %s", contentType)
		}
	}

	key := fmt.Sprintf("uploads/%s/%s%s", userID, uuid.New().String(), filepath.Ext(filename))

	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		CacheControl:  aws.String("private, max-age=86400"),
	})
	if err != nil {
		return "", fmt.Errorf("r2 put: %w", err)
	}

	return key, nil
}

// PresignGet returns a short-lived URL the frontend (or the model) can use to read the image.
func (r *R2Client) PresignGet(ctx context.Context, key string) (string, error) {
	presigner := s3.NewPresignClient(r.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(PresignTTL))
	if err != nil {
		return "", fmt.Errorf("r2 presign: %w", err)
	}
	return req.URL, nil
}

// DeleteImage removes an object. Called if the parent message is deleted.
func (r *R2Client) DeleteImage(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	return err
}
