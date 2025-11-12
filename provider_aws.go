package uploader

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/goliatone/go-print"
)

var (
	_ Uploader        = &AWSProvider{}
	_ ChunkedUploader = &AWSProvider{}
)

type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateMultipartUpload(ctx context.Context, params *s3.CreateMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(ctx context.Context, params *s3.UploadPartInput, optFns ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(ctx context.Context, params *s3.CompleteMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(ctx context.Context, params *s3.AbortMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
	Options() s3.Options
}

type s3PresignClient interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

const awsUploadIDKey = "aws_upload_id"

type AWSProvider struct {
	client    s3API
	bucket    string
	basePath  string
	presigner s3PresignClient
	logger    Logger
	now       func() time.Time
}

func NewAWSProvider(client *s3.Client, bucket string) *AWSProvider {
	return &AWSProvider{
		client:    client,
		bucket:    bucket,
		logger:    &DefaultLogger{},
		presigner: s3.NewPresignClient(client),
		now:       time.Now,
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

func (p *AWSProvider) InitiateChunked(ctx context.Context, session *ChunkSession) (*ChunkSession, error) {
	if session == nil {
		return nil, fmt.Errorf("aws provider: chunk session is nil")
	}

	input := &s3.CreateMultipartUploadInput{
		Bucket: p.bucketPtr(),
		Key:    p.getKey(session.Key),
		ACL:    types.ObjectCannedACLPrivate,
	}

	if session.Metadata != nil {
		if session.Metadata.ContentType != "" {
			input.ContentType = aws.String(session.Metadata.ContentType)
		}
		if session.Metadata.CacheControl != "" {
			input.CacheControl = aws.String(session.Metadata.CacheControl)
		}
	}

	resp, err := p.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("aws provider: create multipart upload: %w", err)
	}

	if session.ProviderData == nil {
		session.ProviderData = make(map[string]any)
	}
	session.ProviderData[awsUploadIDKey] = aws.ToString(resp.UploadId)

	return session, nil
}

func (p *AWSProvider) UploadChunk(ctx context.Context, session *ChunkSession, index int, payload io.Reader) (ChunkPart, error) {
	uploadID, err := p.getUploadID(session)
	if err != nil {
		return ChunkPart{}, err
	}

	if payload == nil {
		return ChunkPart{}, fmt.Errorf("aws provider: chunk payload is nil")
	}

	data, err := io.ReadAll(payload)
	if err != nil {
		return ChunkPart{}, fmt.Errorf("aws provider: read chunk payload: %w", err)
	}

	partNumber := int32(index + 1)
	resp, err := p.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:     p.bucketPtr(),
		Key:        p.getKey(session.Key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
		Body:       bytes.NewReader(data),
	})
	if err != nil {
		return ChunkPart{}, fmt.Errorf("aws provider: upload part: %w", err)
	}

	return ChunkPart{
		Index:      index,
		Size:       int64(len(data)),
		ETag:       aws.ToString(resp.ETag),
		UploadedAt: p.timeNow(),
	}, nil
}

func (p *AWSProvider) CompleteChunked(ctx context.Context, session *ChunkSession) (*FileMeta, error) {
	uploadID, err := p.getUploadID(session)
	if err != nil {
		return nil, err
	}

	completedParts, err := buildCompletedParts(session)
	if err != nil {
		return nil, err
	}

	_, err = p.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   p.bucketPtr(),
		Key:      p.getKey(session.Key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("aws provider: complete multipart upload: %w", err)
	}

	meta := &FileMeta{
		Name:         session.Key,
		OriginalName: session.Key,
		Size:         session.TotalSize,
		URL:          p.getURL(session.Key),
	}

	if session.Metadata != nil {
		meta.ContentType = session.Metadata.ContentType
	}

	return meta, nil
}

func (p *AWSProvider) AbortChunked(ctx context.Context, session *ChunkSession) error {
	uploadID, err := p.getUploadID(session)
	if err != nil {
		return err
	}

	_, err = p.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   p.bucketPtr(),
		Key:      p.getKey(session.Key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return fmt.Errorf("aws provider: abort multipart upload: %w", err)
	}

	return nil
}

func (p *AWSProvider) CreatePresignedPost(ctx context.Context, key string, metadata *Metadata) (*PresignedPost, error) {
	if metadata == nil {
		metadata = &Metadata{}
	}

	opts := p.client.Options()
	if opts.Credentials == nil {
		return nil, fmt.Errorf("aws provider: credentials provider not configured")
	}

	creds, err := opts.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("aws provider: retrieve credentials: %w", err)
	}

	now := p.timeNow().UTC()
	region := opts.Region
	if region == "" {
		region = "us-east-1"
	}

	finalKey := aws.ToString(p.getKey(key))
	credential := fmt.Sprintf("%s/%s/%s/s3/aws4_request",
		creds.AccessKeyID,
		now.Format("20060102"),
		region,
	)

	algorithm := "AWS4-HMAC-SHA256"
	amzDate := now.Format("20060102T150405Z")
	acl := "private"
	if metadata.Public {
		acl = "public-read"
	}

	conditions := []any{
		map[string]string{"bucket": p.bucket},
		map[string]string{"key": finalKey},
		map[string]string{"acl": acl},
		map[string]string{"x-amz-algorithm": algorithm},
		map[string]string{"x-amz-credential": credential},
		map[string]string{"x-amz-date": amzDate},
		[]string{"content-length-range", "1", strconv.FormatInt(DefaultPresignedMaxFileSize, 10)},
	}

	if metadata.ContentType != "" {
		conditions = append(conditions, map[string]string{"Content-Type": metadata.ContentType})
	}

	if metadata.CacheControl != "" {
		conditions = append(conditions, map[string]string{"Cache-Control": metadata.CacheControl})
	}

	if creds.SessionToken != "" {
		conditions = append(conditions, map[string]string{"x-amz-security-token": creds.SessionToken})
	}

	expiry := now.Add(metadata.TTL)

	policyDoc := map[string]any{
		"expiration": expiry.Format(time.RFC3339),
		"conditions": conditions,
	}

	policyJSON, err := json.Marshal(policyDoc)
	if err != nil {
		return nil, fmt.Errorf("aws provider: marshal policy: %w", err)
	}

	policyBase64 := base64.StdEncoding.EncodeToString(policyJSON)
	signingKey := deriveSigningKey(creds.SecretAccessKey, now.Format("20060102"), region)
	signature := hex.EncodeToString(hmacSHA256(signingKey, policyBase64))

	fields := map[string]string{
		"key":                   finalKey,
		"acl":                   acl,
		"Policy":                policyBase64,
		"X-Amz-Algorithm":       algorithm,
		"X-Amz-Credential":      credential,
		"X-Amz-Date":            amzDate,
		"X-Amz-Signature":       signature,
		"success_action_status": "201",
	}

	if metadata.ContentType != "" {
		fields["Content-Type"] = metadata.ContentType
	}
	if metadata.CacheControl != "" {
		fields["Cache-Control"] = metadata.CacheControl
	}
	if creds.SessionToken != "" {
		fields["X-Amz-Security-Token"] = creds.SessionToken
	}

	endpoint := p.buildBucketEndpoint(region)

	return &PresignedPost{
		URL:    endpoint,
		Method: "POST",
		Fields: fields,
		Expiry: expiry,
	}, nil
}

func (p *AWSProvider) bucketPtr() *string {
	return aws.String(p.bucket)
}

func (p *AWSProvider) getUploadID(session *ChunkSession) (string, error) {
	if session == nil {
		return "", fmt.Errorf("aws provider: chunk session is nil")
	}

	if session.ProviderData == nil {
		return "", fmt.Errorf("aws provider: chunk session missing provider data")
	}

	rawID, ok := session.ProviderData[awsUploadIDKey]
	if !ok {
		return "", fmt.Errorf("aws provider: upload id not found in session")
	}

	uploadID, ok := rawID.(string)
	if !ok || uploadID == "" {
		return "", fmt.Errorf("aws provider: invalid upload id stored in session")
	}

	return uploadID, nil
}

func buildCompletedParts(session *ChunkSession) ([]types.CompletedPart, error) {
	if session == nil {
		return nil, fmt.Errorf("chunk session is nil")
	}

	if len(session.UploadedParts) == 0 {
		return nil, fmt.Errorf("no uploaded parts recorded for session %s", session.ID)
	}

	parts := make([]types.CompletedPart, 0, len(session.UploadedParts))
	for _, part := range session.UploadedParts {
		if part.ETag == "" {
			return nil, fmt.Errorf("missing ETag for part %d", part.Index)
		}

		partNumber := int32(part.Index + 1)
		partEntry := types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(partNumber),
		}
		parts = append(parts, partEntry)
	}

	sort.Slice(parts, func(i, j int) bool {
		return aws.ToInt32(parts[i].PartNumber) < aws.ToInt32(parts[j].PartNumber)
	})

	return parts, nil
}

func (p *AWSProvider) buildBucketEndpoint(region string) string {
	host := fmt.Sprintf("%s.s3.%s.amazonaws.com", p.bucket, region)
	if region == "" || region == "us-east-1" {
		host = fmt.Sprintf("%s.s3.amazonaws.com", p.bucket)
	}
	u := url.URL{
		Scheme: "https",
		Host:   host,
	}
	return u.String()
}

func (p *AWSProvider) timeNow() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func deriveSigningKey(secret, dateStamp, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}
