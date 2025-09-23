package uploader

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type mockProvider struct {
	uploadFunc       func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error)
	getFunc          func(ctx context.Context, path string) ([]byte, error)
	deleteFunc       func(ctx context.Context, path string) error
	getPresignedFunc func(ctx context.Context, path string, expires time.Duration) (string, error)
	validateFunc     func(ctx context.Context) error
	shouldValidate   bool
}

func (m *mockProvider) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, path, content, opts...)
	}
	return "http://example.com/" + path, nil
}

func (m *mockProvider) GetFile(ctx context.Context, path string) ([]byte, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, path)
	}
	return []byte("mock content"), nil
}

func (m *mockProvider) DeleteFile(ctx context.Context, path string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, path)
	}
	return nil
}

func (m *mockProvider) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	if m.getPresignedFunc != nil {
		return m.getPresignedFunc(ctx, path, expires)
	}
	return "http://example.com/presigned/" + path, nil
}

func (m *mockProvider) Validate(ctx context.Context) error {
	if m.shouldValidate && m.validateFunc != nil {
		return m.validateFunc(ctx)
	}
	return nil
}

func TestNewMultiProvider(t *testing.T) {
	localProvider := NewFSProvider("/tmp")
	objectStore := &mockProvider{}

	provider := NewMultiProvider(localProvider, objectStore)

	if provider == nil {
		t.Fatal("NewMultiProvider returned nil")
	}

	if provider.local != localProvider {
		t.Error("Local provider not set correctly")
	}

	if provider.objectStore != objectStore {
		t.Error("Object store not set correctly")
	}

	if provider.logger == nil {
		t.Error("Provider should have default logger")
	}
}

func TestMultiProviderWithLogger(t *testing.T) {
	localProvider := NewFSProvider("/tmp")
	objectStore := &mockProvider{}
	logger := &mockLogger{}

	provider := NewMultiProvider(localProvider, objectStore).WithLogger(logger)

	if provider.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestMultiProviderUploadFile(t *testing.T) {
	t.Run("object store upload failure", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			uploadFunc: func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
				return "", errors.New("object store upload failed")
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		_, err = provider.UploadFile(context.Background(), "test.jpg", []byte("test content"))
		if err == nil {
			t.Fatal("Expected error from object store upload failure")
		}

		if err.Error() != "object store upload failed" {
			t.Errorf("Expected 'object store upload failed', got '%s'", err.Error())
		}
	})

	t.Run("successful upload flow", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			uploadFunc: func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
				return "http://example.com/" + path, nil
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		url, err := provider.UploadFile(context.Background(), "test.jpg", []byte("test content"))
		if err != nil {
			t.Fatalf("UploadFile failed: %v", err)
		}

		if url != "http://example.com/test.jpg" {
			t.Errorf("Expected URL 'http://example.com/test.jpg', got '%s'", url)
		}
	})
}

func TestMultiProviderGetFile(t *testing.T) {
	t.Run("successful get from local", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			getFunc: func(ctx context.Context, path string) ([]byte, error) {
				t.Error("Object store get should not be called when local succeeds")
				return nil, nil
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		err = os.WriteFile(tmpDir+"/test.jpg", []byte("local content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		content, err := provider.GetFile(context.Background(), "test.jpg")
		if err != nil {
			t.Fatalf("GetFile failed: %v", err)
		}

		if string(content) != "local content" {
			t.Errorf("Expected content 'local content', got '%s'", string(content))
		}
	})

	t.Run("fallback to object store", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			getFunc: func(ctx context.Context, path string) ([]byte, error) {
				if path != "test.jpg" {
					t.Errorf("Expected path 'test.jpg', got '%s'", path)
				}
				return []byte("object store content"), nil
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		content, err := provider.GetFile(context.Background(), "test.jpg")
		if err != nil {
			t.Fatalf("GetFile failed: %v", err)
		}

		if string(content) != "object store content" {
			t.Errorf("Expected content 'object store content', got '%s'", string(content))
		}
	})

	t.Run("both providers fail", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			getFunc: func(ctx context.Context, path string) ([]byte, error) {
				return nil, errors.New("object store error")
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		_, err = provider.GetFile(context.Background(), "nonexistent.jpg")
		if err == nil {
			t.Fatal("Expected error when both providers fail")
		}

		if err.Error() != "object store error" {
			t.Errorf("Expected 'object store error', got '%s'", err.Error())
		}
	})
}

func TestMultiProviderDeleteFile(t *testing.T) {
	t.Run("successful delete flow", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectDeleted := false
		objectStore := &mockProvider{
			deleteFunc: func(ctx context.Context, path string) error {
				objectDeleted = true
				if path != "test.jpg" {
					t.Errorf("Expected path 'test.jpg', got '%s'", path)
				}
				return nil
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		err = provider.DeleteFile(context.Background(), "test.jpg")
		if err != nil {
			t.Fatalf("DeleteFile failed: %v", err)
		}

		if !objectDeleted {
			t.Error("Object store delete was not called")
		}
	})

	t.Run("object store delete failure", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			deleteFunc: func(ctx context.Context, path string) error {
				return errors.New("object store delete failed")
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		err = provider.DeleteFile(context.Background(), "test.jpg")
		if err == nil {
			t.Fatal("Expected error from object store delete failure")
		}

		if err.Error() != "object store delete failed" {
			t.Errorf("Expected 'object store delete failed', got '%s'", err.Error())
		}
	})
}

func TestMultiProviderGetPresignedURL(t *testing.T) {
	t.Run("successful presigned URL", func(t *testing.T) {
		localProvider := NewFSProvider("/tmp")
		objectStore := &mockProvider{
			getPresignedFunc: func(ctx context.Context, path string, expires time.Duration) (string, error) {
				if path != "test.jpg" {
					t.Errorf("Expected path 'test.jpg', got '%s'", path)
				}
				if expires != time.Hour {
					t.Errorf("Expected expires '1h', got '%v'", expires)
				}
				return "http://example.com/presigned/test.jpg", nil
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		url, err := provider.GetPresignedURL(context.Background(), "test.jpg", time.Hour)
		if err != nil {
			t.Fatalf("GetPresignedURL failed: %v", err)
		}

		if url != "http://example.com/presigned/test.jpg" {
			t.Errorf("Expected URL 'http://example.com/presigned/test.jpg', got '%s'", url)
		}
	})

	t.Run("presigned URL failure", func(t *testing.T) {
		localProvider := NewFSProvider("/tmp")
		objectStore := &mockProvider{
			getPresignedFunc: func(ctx context.Context, path string, expires time.Duration) (string, error) {
				return "", errors.New("presign failed")
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		_, err := provider.GetPresignedURL(context.Background(), "test.jpg", time.Hour)
		if err == nil {
			t.Fatal("Expected error from presign failure")
		}

		if err.Error() != "presign failed" {
			t.Errorf("Expected 'presign failed', got '%s'", err.Error())
		}
	})
}

func TestMultiProviderValidate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			shouldValidate: true,
			validateFunc: func(ctx context.Context) error {
				return nil
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		err = provider.Validate(context.Background())
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
	})

	t.Run("nil local provider", func(t *testing.T) {
		objectStore := &mockProvider{}
		provider := NewMultiProvider(nil, objectStore)

		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for nil local provider")
		}

		if err.Error() != "multi provider: local provider not configured" {
			t.Errorf("Expected 'multi provider: local provider not configured', got '%s'", err.Error())
		}
	})

	t.Run("local provider validation failure", func(t *testing.T) {
		localProvider := NewFSProvider("/nonexistent")
		objectStore := &mockProvider{}
		provider := NewMultiProvider(localProvider, objectStore)

		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error from local provider validation failure")
		}

		if err.Error() == "" {
			t.Error("Expected non-empty error message")
		}
	})

	t.Run("nil object store", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		provider := NewMultiProvider(localProvider, nil)

		err = provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for nil object store")
		}

		if err.Error() != "multi provider: object store not configured" {
			t.Errorf("Expected 'multi provider: object store not configured', got '%s'", err.Error())
		}
	})

	t.Run("object store validation failure", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{
			shouldValidate: true,
			validateFunc: func(ctx context.Context) error {
				return errors.New("object store validation failed")
			},
		}

		provider := NewMultiProvider(localProvider, objectStore)

		err = provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error from object store validation failure")
		}

		if err.Error() != "multi provider: object store validation failed: object store validation failed" {
			t.Errorf("Expected 'multi provider: object store validation failed: object store validation failed', got '%s'", err.Error())
		}
	})

	t.Run("providers without validation", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "multi-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		localProvider := NewFSProvider(tmpDir)
		objectStore := &mockProvider{shouldValidate: false}
		provider := NewMultiProvider(localProvider, objectStore)

		err = provider.Validate(context.Background())
		if err != nil {
			t.Fatalf("Validate should succeed for providers without validation: %v", err)
		}
	})
}

func TestValidateOptional(t *testing.T) {
	t.Run("provider with validation", func(t *testing.T) {
		provider := &mockProvider{
			shouldValidate: true,
			validateFunc: func(ctx context.Context) error {
				return nil
			},
		}

		err := validateOptional(context.Background(), provider)
		if err != nil {
			t.Fatalf("validateOptional failed: %v", err)
		}
	})

	t.Run("provider without validation", func(t *testing.T) {
		provider := &mockProvider{shouldValidate: false}

		err := validateOptional(context.Background(), provider)
		if err != nil {
			t.Fatalf("validateOptional should succeed for provider without validation: %v", err)
		}
	})

	t.Run("provider validation failure", func(t *testing.T) {
		provider := &mockProvider{
			shouldValidate: true,
			validateFunc: func(ctx context.Context) error {
				return errors.New("validation error")
			},
		}

		err := validateOptional(context.Background(), provider)
		if err == nil {
			t.Fatal("Expected error from provider validation failure")
		}

		if err.Error() != "validation error" {
			t.Errorf("Expected 'validation error', got '%s'", err.Error())
		}
	})
}

func TestMultiProviderInterface(t *testing.T) {
	var _ Uploader = &MultiProvider{}
	var _ ProviderValidator = &MultiProvider{}
}