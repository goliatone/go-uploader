package uploader

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
	"time"

	gerrors "github.com/goliatone/go-errors"
)

type mockUploader struct {
	uploadFunc       func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error)
	getFunc          func(ctx context.Context, path string) ([]byte, error)
	deleteFunc       func(ctx context.Context, path string) error
	getPresignedFunc func(ctx context.Context, path string, expires time.Duration) (string, error)
	validateFunc     func(ctx context.Context) error
	shouldValidate   bool
}

func (m *mockUploader) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, path, content, opts...)
	}
	return "http://example.com/" + path, nil
}

func (m *mockUploader) GetFile(ctx context.Context, path string) ([]byte, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, path)
	}
	return []byte("mock file content"), nil
}

func (m *mockUploader) DeleteFile(ctx context.Context, path string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, path)
	}
	return nil
}

func (m *mockUploader) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	if m.getPresignedFunc != nil {
		return m.getPresignedFunc(ctx, path, expires)
	}
	return "http://example.com/presigned/" + path, nil
}

func (m *mockUploader) Validate(ctx context.Context) error {
	if m.shouldValidate && m.validateFunc != nil {
		return m.validateFunc(ctx)
	}
	return nil
}

type mockLogger struct {
	infoMessages  []string
	errorMessages []string
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.infoMessages = append(l.infoMessages, msg)
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.errorMessages = append(l.errorMessages, msg)
}

func TestNewManager(t *testing.T) {
	manager := NewManager()

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.logger == nil {
		t.Error("Manager should have default logger")
	}

	if manager.validator == nil {
		t.Error("Manager should have default validator")
	}
}

func TestManagerWithOptions(t *testing.T) {
	mockUploader := &mockUploader{}
	mockLogger := &mockLogger{}
	mockValidator := NewValidator()

	manager := NewManager(
		WithProvider(mockUploader),
		WithLogger(mockLogger),
		WithValidator(mockValidator),
	)

	if manager.provider != mockUploader {
		t.Error("Provider not set correctly")
	}

	if manager.logger != mockLogger {
		t.Error("Logger not set correctly")
	}

	if manager.validator != mockValidator {
		t.Error("Validator not set correctly")
	}
}

func TestManagerUploadFile(t *testing.T) {
	mockUploader := &mockUploader{
		uploadFunc: func(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
			if path != "test.jpg" {
				t.Errorf("Expected path 'test.jpg', got '%s'", path)
			}
			if string(content) != "test content" {
				t.Errorf("Expected content 'test content', got '%s'", string(content))
			}
			return "http://example.com/test.jpg", nil
		},
	}

	manager := NewManager(WithProvider(mockUploader))

	url, err := manager.UploadFile(context.Background(), "test.jpg", []byte("test content"))
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	if url != "http://example.com/test.jpg" {
		t.Errorf("Expected URL 'http://example.com/test.jpg', got '%s'", url)
	}
}

func TestManagerUploadFileWithoutProvider(t *testing.T) {
	manager := NewManager()

	_, err := manager.UploadFile(context.Background(), "test.jpg", []byte("test content"))
	if err == nil {
		t.Fatal("Expected error when no provider is set")
	}

	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("Expected ErrProviderNotConfigured, got %v", err)
	}
}

func TestManagerGetFile(t *testing.T) {
	expectedContent := []byte("mock file content")
	mockUploader := &mockUploader{
		getFunc: func(ctx context.Context, path string) ([]byte, error) {
			if path != "test.jpg" {
				t.Errorf("Expected path 'test.jpg', got '%s'", path)
			}
			return expectedContent, nil
		},
	}

	manager := NewManager(WithProvider(mockUploader))

	content, err := manager.GetFile(context.Background(), "test.jpg")
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}

	if !bytes.Equal(content, expectedContent) {
		t.Errorf("Expected content '%s', got '%s'", expectedContent, content)
	}
}

func TestManagerDeleteFile(t *testing.T) {
	mockUploader := &mockUploader{
		deleteFunc: func(ctx context.Context, path string) error {
			if path != "test.jpg" {
				t.Errorf("Expected path 'test.jpg', got '%s'", path)
			}
			return nil
		},
	}

	manager := NewManager(WithProvider(mockUploader))

	err := manager.DeleteFile(context.Background(), "test.jpg")
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
}

func TestManagerGetPresignedURL(t *testing.T) {
	mockUploader := &mockUploader{
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

	manager := NewManager(WithProvider(mockUploader))

	url, err := manager.GetPresignedURL(context.Background(), "test.jpg", time.Hour)
	if err != nil {
		t.Fatalf("GetPresignedURL failed: %v", err)
	}

	if url != "http://example.com/presigned/test.jpg" {
		t.Errorf("Expected URL 'http://example.com/presigned/test.jpg', got '%s'", url)
	}
}

func TestManagerValidateProvider(t *testing.T) {
	t.Run("valid provider", func(t *testing.T) {
		mockUploader := &mockUploader{
			shouldValidate: true,
			validateFunc: func(ctx context.Context) error {
				return nil
			},
		}

		manager := NewManager(WithProvider(mockUploader))

		err := manager.ValidateProvider(context.Background())
		if err != nil {
			t.Fatalf("ValidateProvider failed: %v", err)
		}
	})

	t.Run("invalid provider", func(t *testing.T) {
		mockUploader := &mockUploader{
			shouldValidate: true,
			validateFunc: func(ctx context.Context) error {
				return errors.New("validation failed")
			},
		}

		manager := NewManager(WithProvider(mockUploader))

		err := manager.ValidateProvider(context.Background())
		if err == nil {
			t.Fatal("Expected validation error")
		}

		if err.Error() != "validation failed" {
			t.Errorf("Expected 'validation failed', got '%s'", err.Error())
		}
	})

	t.Run("no provider", func(t *testing.T) {
		manager := NewManager()

		err := manager.ValidateProvider(context.Background())
		if err == nil {
			t.Fatal("Expected error when no provider is set")
		}

		if !errors.Is(err, ErrProviderNotConfigured) {
			t.Errorf("Expected ErrProviderNotConfigured, got %v", err)
		}
	})
}

func createMultipartFileHeader(filename, contentType string, content []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)

	part, _ := writer.CreatePart(h)
	part.Write(content)
	writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(32 << 20)

	return form.File["file"][0]
}

func TestManagerHandleFile(t *testing.T) {
	t.Run("successful file handling", func(t *testing.T) {
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		content := append(pngHeader, []byte("mock png content")...)

		fileHeader := createMultipartFileHeader("test.png", "image/png", content)

		mockUploader := &mockUploader{
			uploadFunc: func(ctx context.Context, path string, fileContent []byte, opts ...UploadOption) (string, error) {
				return "http://example.com/" + path, nil
			},
		}

		manager := NewManager(WithProvider(mockUploader))

		meta, err := manager.HandleFile(context.Background(), fileHeader, "uploads")
		if err != nil {
			t.Fatalf("HandleFile failed: %v", err)
		}

		if meta == nil {
			t.Fatal("Expected non-nil FileMeta")
		}

		if meta.OriginalName != "test.png" {
			t.Errorf("Expected original name 'test.png', got '%s'", meta.OriginalName)
		}

		if meta.ContentType != "image/png" {
			t.Errorf("Expected content type 'image/png', got '%s'", meta.ContentType)
		}

		if meta.Size != fileHeader.Size {
			t.Errorf("Expected size %d, got %d", fileHeader.Size, meta.Size)
		}

		if !strings.Contains(meta.URL, "http://example.com/") {
			t.Errorf("Expected URL to contain 'http://example.com/', got '%s'", meta.URL)
		}
	})

	t.Run("nil file", func(t *testing.T) {
		manager := NewManager()

		_, err := manager.HandleFile(context.Background(), nil, "uploads")
		if err == nil {
			t.Fatal("Expected error for nil file")
		}

		if !gerrors.IsNotFound(err) {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	t.Run("file validation failure", func(t *testing.T) {
		content := []byte("invalid content")
		fileHeader := createMultipartFileHeader("test.txt", "text/plain", content)

		manager := NewManager()

		_, err := manager.HandleFile(context.Background(), fileHeader, "uploads")
		if err == nil {
			t.Fatal("Expected validation error")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})
}

func TestUploadOptions(t *testing.T) {
	metadata := &Metadata{}

	WithContentType("image/jpeg")(metadata)
	if metadata.ContentType != "image/jpeg" {
		t.Errorf("Expected ContentType 'image/jpeg', got '%s'", metadata.ContentType)
	}

	WithCacheControl("max-age=3600")(metadata)
	if metadata.CacheControl != "max-age=3600" {
		t.Errorf("Expected CacheControl 'max-age=3600', got '%s'", metadata.CacheControl)
	}

	WithPublicAccess(true)(metadata)
	if !metadata.Public {
		t.Error("Expected Public to be true")
	}

	WithTTL(time.Hour)(metadata)
	if metadata.TTL != time.Hour {
		t.Errorf("Expected TTL '1h', got '%v'", metadata.TTL)
	}
}

func TestManagerProviderValidationContext(t *testing.T) {
	validateCalled := false
	mockUploader := &mockUploader{
		shouldValidate: true,
		validateFunc: func(ctx context.Context) error {
			validateCalled = true
			return nil
		},
	}

	ctx := context.WithValue(context.Background(), "test", "value")
	manager := NewManager(
		WithProviderValidationContext(ctx),
		WithProvider(mockUploader),
	)

	if !validateCalled {
		t.Error("Expected validation to be called during provider setup")
	}

	_, err := manager.UploadFile(context.Background(), "test.jpg", []byte("content"))
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
}
