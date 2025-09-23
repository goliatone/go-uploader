package uploader

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
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

func TestAWSProviderInterface(t *testing.T) {
	var _ Uploader = &AWSProvider{}
	var _ ProviderValidator = &AWSProvider{}
}