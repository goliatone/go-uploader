package uploader

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestAWSProviderValidate(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		provider := &AWSProvider{
			client: nil,
			bucket: "test-bucket",
		}

		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for nil client")
		}

		if !strings.Contains(err.Error(), "client not configured") {
			t.Errorf("Expected error message to contain 'client not configured', got '%s'", err.Error())
		}
	})

	t.Run("empty bucket", func(t *testing.T) {
		provider := &AWSProvider{
			client: &s3.Client{},
			bucket: "",
		}

		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for empty bucket")
		}

		if !strings.Contains(err.Error(), "bucket not configured") {
			t.Errorf("Expected error message to contain 'bucket not configured', got '%s'", err.Error())
		}
	})
}

func TestAWSProviderGetKey(t *testing.T) {
	t.Run("without base path", func(t *testing.T) {
		provider := &AWSProvider{
			bucket:   "test-bucket",
			basePath: "",
		}
		key := provider.getKey("test.jpg")

		if *key != "test.jpg" {
			t.Errorf("Expected key 'test.jpg', got '%s'", *key)
		}
	})

	t.Run("with base path", func(t *testing.T) {
		provider := &AWSProvider{
			bucket:   "test-bucket",
			basePath: "uploads",
		}
		key := provider.getKey("test.jpg")

		if *key != "uploads/test.jpg" {
			t.Errorf("Expected key 'uploads/test.jpg', got '%s'", *key)
		}
	})
}

func TestAWSProviderGetURL(t *testing.T) {
	t.Run("without base path", func(t *testing.T) {
		provider := &AWSProvider{
			bucket:   "test-bucket",
			basePath: "",
		}
		url := provider.getURL("test.jpg")

		if url != "/test.jpg" {
			t.Errorf("Expected URL '/test.jpg', got '%s'", url)
		}
	})

	t.Run("with base path", func(t *testing.T) {
		provider := &AWSProvider{
			bucket:   "test-bucket",
			basePath: "uploads",
		}
		url := provider.getURL("test.jpg")

		if url != "/uploads/test.jpg" {
			t.Errorf("Expected URL '/uploads/test.jpg', got '%s'", url)
		}
	})

	t.Run("with leading slash", func(t *testing.T) {
		provider := &AWSProvider{
			bucket:   "test-bucket",
			basePath: "",
		}
		url := provider.getURL("/test.jpg")

		if url != "//test.jpg" {
			t.Errorf("Expected URL '//test.jpg', got '%s'", url)
		}
	})
}

func TestAWSProviderChaining(t *testing.T) {
	logger := &mockLogger{}

	provider := &AWSProvider{
		bucket: "test-bucket",
		logger: &DefaultLogger{},
	}

	result := provider.WithLogger(logger)
	if result != provider {
		t.Error("WithLogger should return the same provider instance")
	}
	if provider.logger != logger {
		t.Error("Logger not set correctly")
	}

	result = provider.WithBasePath("uploads")
	if result != provider {
		t.Error("WithBasePath should return the same provider instance")
	}
	if provider.basePath != "uploads" {
		t.Error("BasePath not set correctly")
	}
}

func TestAWSProviderCreatePresignedPost(t *testing.T) {
	ctx := context.Background()
	client := &fakeS3Client{
		options: s3.Options{
			Region: "us-east-1",
			Credentials: aws.NewCredentialsCache(staticCredentialsProvider{
				creds: aws.Credentials{
					AccessKeyID:     "AKIA123456789",
					SecretAccessKey: "secret",
					SessionToken:    "session-token",
				},
			}),
		},
	}

	provider := NewAWSProvider(&s3.Client{}, "test-bucket")
	provider.client = client
	provider.presigner = s3.NewPresignClient(&s3.Client{})
	provider.now = func() time.Time {
		return time.Unix(1700000000, 0)
	}

	post, err := provider.CreatePresignedPost(ctx, "uploads/test.jpg", &Metadata{
		ContentType: "image/jpeg",
		TTL:         10 * time.Minute,
		Public:      true,
	})
	if err != nil {
		t.Fatalf("CreatePresignedPost returned error: %v", err)
	}

	if post.Method != "POST" {
		t.Fatalf("expected POST method, got %s", post.Method)
	}

	if post.Fields["key"] != "uploads/test.jpg" {
		t.Fatalf("expected key field to be uploads/test.jpg, got %s", post.Fields["key"])
	}

	if post.Fields["Policy"] == "" || post.Fields["X-Amz-Signature"] == "" {
		t.Fatalf("expected policy and signature fields to be populated")
	}
}

type mockAWSProvider struct {
	*AWSProvider
	uploadFunc       func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error)
	getFunc          func(ctx context.Context, path string) ([]byte, error)
	deleteFunc       func(ctx context.Context, path string) error
	getPresignedFunc func(ctx context.Context, path string, expires time.Duration) (string, error)
}

func (m *mockAWSProvider) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, path, content, opts...)
	}
	return m.AWSProvider.UploadFile(ctx, path, content, opts...)
}

func (m *mockAWSProvider) GetFile(ctx context.Context, path string) ([]byte, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, path)
	}
	return m.AWSProvider.GetFile(ctx, path)
}

func (m *mockAWSProvider) DeleteFile(ctx context.Context, path string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, path)
	}
	return m.AWSProvider.DeleteFile(ctx, path)
}

func (m *mockAWSProvider) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	if m.getPresignedFunc != nil {
		return m.getPresignedFunc(ctx, path, expires)
	}
	return m.AWSProvider.GetPresignedURL(ctx, path, expires)
}

func TestAWSProviderOperations(t *testing.T) {
	t.Run("upload file operations", func(t *testing.T) {
		provider := &mockAWSProvider{
			AWSProvider: &AWSProvider{bucket: "test-bucket"},
			uploadFunc: func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
				if path == "error.jpg" {
					return "", errors.New("upload failed")
				}
				return "/uploaded/" + path, nil
			},
		}

		url, err := provider.UploadFile(context.Background(), "test.jpg", []byte("content"))
		if err != nil {
			t.Fatalf("UploadFile failed: %v", err)
		}
		if url != "/uploaded/test.jpg" {
			t.Errorf("Expected URL '/uploaded/test.jpg', got '%s'", url)
		}

		_, err = provider.UploadFile(context.Background(), "error.jpg", []byte("content"))
		if err == nil {
			t.Fatal("Expected error for error.jpg")
		}
	})

	t.Run("get file operations", func(t *testing.T) {
		provider := &mockAWSProvider{
			AWSProvider: &AWSProvider{bucket: "test-bucket"},
			getFunc: func(ctx context.Context, path string) ([]byte, error) {
				if path == "error.jpg" {
					return nil, errors.New("get failed")
				}
				return []byte("file content"), nil
			},
		}

		content, err := provider.GetFile(context.Background(), "test.jpg")
		if err != nil {
			t.Fatalf("GetFile failed: %v", err)
		}
		if string(content) != "file content" {
			t.Errorf("Expected content 'file content', got '%s'", string(content))
		}

		_, err = provider.GetFile(context.Background(), "error.jpg")
		if err == nil {
			t.Fatal("Expected error for error.jpg")
		}
	})

	t.Run("delete file operations", func(t *testing.T) {
		provider := &mockAWSProvider{
			AWSProvider: &AWSProvider{bucket: "test-bucket"},
			deleteFunc: func(ctx context.Context, path string) error {
				if path == "error.jpg" {
					return errors.New("delete failed")
				}
				return nil
			},
		}

		err := provider.DeleteFile(context.Background(), "test.jpg")
		if err != nil {
			t.Fatalf("DeleteFile failed: %v", err)
		}

		err = provider.DeleteFile(context.Background(), "error.jpg")
		if err == nil {
			t.Fatal("Expected error for error.jpg")
		}
	})

	t.Run("get presigned url operations", func(t *testing.T) {
		provider := &mockAWSProvider{
			AWSProvider: &AWSProvider{bucket: "test-bucket"},
			getPresignedFunc: func(ctx context.Context, path string, expires time.Duration) (string, error) {
				if path == "error.jpg" {
					return "", errors.New("presign failed")
				}
				return "https://presigned.example.com/" + path, nil
			},
		}

		url, err := provider.GetPresignedURL(context.Background(), "test.jpg", time.Hour)
		if err != nil {
			t.Fatalf("GetPresignedURL failed: %v", err)
		}
		if url != "https://presigned.example.com/test.jpg" {
			t.Errorf("Expected URL 'https://presigned.example.com/test.jpg', got '%s'", url)
		}

		_, err = provider.GetPresignedURL(context.Background(), "error.jpg", time.Hour)
		if err == nil {
			t.Fatal("Expected error for error.jpg")
		}
	})
}

func TestAWSProviderChunkedLifecycle(t *testing.T) {
	ctx := context.Background()
	client := &fakeS3Client{
		createMultipartOutput: &s3.CreateMultipartUploadOutput{
			UploadId: aws.String("upload-123"),
		},
		uploadPartOutput: &s3.UploadPartOutput{
			ETag: aws.String("etag-0"),
		},
		completeMultipartOutput: &s3.CompleteMultipartUploadOutput{},
		abortMultipartOutput:    &s3.AbortMultipartUploadOutput{},
	}

	provider := &AWSProvider{
		client:    client,
		bucket:    "test-bucket",
		logger:    &DefaultLogger{},
		presigner: s3.NewPresignClient(&s3.Client{}),
	}

	session := &ChunkSession{
		ID:            "aws-session",
		Key:           "chunks/aws.bin",
		TotalSize:     4,
		UploadedParts: make(map[int]ChunkPart),
		Metadata: &Metadata{
			ContentType: "application/octet-stream",
		},
	}

	if _, err := provider.InitiateChunked(ctx, session); err != nil {
		t.Fatalf("InitiateChunked failed: %v", err)
	}

	part, err := provider.UploadChunk(ctx, session, 0, bytes.NewReader([]byte("data")))
	if err != nil {
		t.Fatalf("UploadChunk failed: %v", err)
	}
	session.UploadedParts[0] = part

	meta, err := provider.CompleteChunked(ctx, session)
	if err != nil {
		t.Fatalf("CompleteChunked failed: %v", err)
	}

	if meta.ContentType != "application/octet-stream" {
		t.Fatalf("expected content type propagated")
	}

	abortSession := &ChunkSession{
		ID:        "aws-session-abort",
		Key:       "chunks/abort.bin",
		TotalSize: 4,
		UploadedParts: map[int]ChunkPart{
			0: {Index: 0, Size: 4, ETag: "etag-0"},
		},
		Metadata: &Metadata{},
	}

	if _, err := provider.InitiateChunked(ctx, abortSession); err != nil {
		t.Fatalf("InitiateChunked for abort session failed: %v", err)
	}

	if err := provider.AbortChunked(ctx, abortSession); err != nil {
		t.Fatalf("AbortChunked failed: %v", err)
	}

	if !client.abortCalled {
		t.Fatalf("expected abort to be invoked on client")
	}
}

type fakeS3Client struct {
	createMultipartOutput   *s3.CreateMultipartUploadOutput
	uploadPartOutput        *s3.UploadPartOutput
	completeMultipartOutput *s3.CompleteMultipartUploadOutput
	abortMultipartOutput    *s3.AbortMultipartUploadOutput
	abortCalled             bool
	lastCompletedParts      []types.CompletedPart
	options                 s3.Options
}

func (f *fakeS3Client) PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("data"))),
	}, nil
}

func (f *fakeS3Client) DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3Client) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}

func (f *fakeS3Client) CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	return f.createMultipartOutput, nil
}

func (f *fakeS3Client) UploadPart(_ context.Context, params *s3.UploadPartInput, _ ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	if params.Body != nil {
		_, _ = io.ReadAll(params.Body)
	}
	return f.uploadPartOutput, nil
}

func (f *fakeS3Client) CompleteMultipartUpload(_ context.Context, params *s3.CompleteMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	if params.MultipartUpload != nil {
		f.lastCompletedParts = params.MultipartUpload.Parts
	}
	return f.completeMultipartOutput, nil
}

func (f *fakeS3Client) AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	f.abortCalled = true
	return f.abortMultipartOutput, nil
}

func (f *fakeS3Client) Options() s3.Options {
	return f.options
}

func TestAWSProviderInterface(t *testing.T) {
	var _ Uploader = &AWSProvider{}
	var _ ProviderValidator = &AWSProvider{}
}

type staticCredentialsProvider struct {
	creds aws.Credentials
}

func (s staticCredentialsProvider) Retrieve(context.Context) (aws.Credentials, error) {
	return s.creds, nil
}
