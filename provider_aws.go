package uploader

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/goliatone/go-print"
)

var _ Uploader = &AWSProvider{}

type AWSProvider struct {
	client    *s3.Client
	bucket    string
	basePath  string
	presigner *s3.PresignClient
	logger    Logger
}

func NewAWSProvider(client *s3.Client, bucket string) *AWSProvider {
	return &AWSProvider{
		client:    client,
		bucket:    bucket,
		logger:    &DefaultLogger{},
		presigner: s3.NewPresignClient(client),
	}
}

func (p *AWSProvider) WithLogger(logger Logger) *AWSProvider {
	p.logger = logger
	return p
}

func (p *AWSProvider) WithBasePath(basePath string) *AWSProvider {
	p.basePath = basePath
	return p
}

func (p *AWSProvider) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	md := &Metadata{}
	for _, opt := range opts {
		opt(md)
	}

	p.logger.Info("upload image", "bucket", p.bucket, "path", path)

	res, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(p.bucket),
		Key:          p.getKey(path),
		Body:         bytes.NewReader(content),
		ContentType:  aws.String(md.ContentType),
		CacheControl: aws.String(md.CacheControl),
		ACL:          types.ObjectCannedACLPrivate,
	})
	if err != nil {
		p.logger.Error("S3 upload failed", err)
		return "", fmt.Errorf("failed to upload image: %w", err)
	}

	p.logger.Info("upload image", "res", print.MaybeHighlightJSON(res))

	return p.getURL(path), nil
}

func (p *AWSProvider) GetFile(ctx context.Context, path string) ([]byte, error) {
	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    p.getKey(path),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(out.Body)
	return buf.Bytes(), err
}

func (p *AWSProvider) DeleteFile(ctx context.Context, path string) error {
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    p.getKey(path),
	})
	return err
}

func (p *AWSProvider) GetPresignedURL(ctx context.Context, path string, ttl time.Duration) (string, error) {
	req, err := p.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    p.getKey(path),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (p *AWSProvider) getKey(key string) *string {
	if p.basePath == "" {
		return aws.String(key)
	}
	return aws.String(path.Join(p.basePath, key))
}

func (p *AWSProvider) getURL(key string) string {
	out := key

	if p.basePath != "" {
		out = path.Join(p.basePath, key)
	}

	if out[1] != '/' {
		out = "/" + out
	}

	return out
}

func (p *AWSProvider) Validate(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("aws provider: client not configured")
	}

	if p.bucket == "" {
		return fmt.Errorf("aws provider: bucket not configured")
	}

	_, err := p.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(p.bucket)})
	if err != nil {
		return fmt.Errorf("aws provider: head bucket: %w", err)
	}

	return nil
}
