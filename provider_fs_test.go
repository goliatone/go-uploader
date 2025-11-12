package uploader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewFSProvider(t *testing.T) {
	base := "/tmp/test"
	provider := NewFSProvider(base)

	if provider == nil {
		t.Fatal("NewFSProvider returned nil")
	}

	if provider.base != base {
		t.Errorf("Expected base '%s', got '%s'", base, provider.base)
	}

	if provider.logger == nil {
		t.Error("Provider should have default logger")
	}

	if provider.root == nil {
		t.Error("Provider should have root filesystem")
	}
}

func TestFSProviderWithLogger(t *testing.T) {
	logger := &mockLogger{}
	provider := NewFSProvider("/tmp").WithLogger(logger)

	if provider.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestFSProviderWithFS(t *testing.T) {
	mockFS := os.DirFS("/tmp")
	provider := NewFSProvider("/tmp").WithFS(mockFS)

	if provider.root != mockFS {
		t.Error("Filesystem not set correctly")
	}
}

func TestFSProviderWithURLPrefix(t *testing.T) {
	t.Run("without trailing slash", func(t *testing.T) {
		prefix := "http://example.com/files"
		provider := NewFSProvider("/tmp").WithURLPrefix(prefix)

		expected := prefix + "/"
		if provider.urlPrefix != expected {
			t.Errorf("Expected URL prefix '%s', got '%s'", expected, provider.urlPrefix)
		}
	})

	t.Run("with trailing slash", func(t *testing.T) {
		prefix := "http://example.com/files/"
		provider := NewFSProvider("/tmp").WithURLPrefix(prefix)

		if provider.urlPrefix != prefix {
			t.Errorf("Expected URL prefix '%s', got '%s'", prefix, provider.urlPrefix)
		}
	})
}

func TestFSProviderUploadFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fs-provider-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider := NewFSProvider(tmpDir)

	t.Run("successful upload", func(t *testing.T) {
		content := []byte("test file content")
		path := "test.jpg"

		url, err := provider.UploadFile(context.Background(), path, content)
		if err != nil {
			t.Fatalf("UploadFile failed: %v", err)
		}

		expectedPath := filepath.Join(tmpDir, path)
		if url != expectedPath {
			t.Errorf("Expected URL '%s', got '%s'", expectedPath, url)
		}

		savedContent, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("Failed to read saved file: %v", err)
		}

		if string(savedContent) != string(content) {
			t.Errorf("Expected content '%s', got '%s'", string(content), string(savedContent))
		}
	})

	t.Run("upload with subdirectory", func(t *testing.T) {
		content := []byte("test content")
		path := "uploads/test.jpg"

		_, err := provider.UploadFile(context.Background(), path, content)
		if err != nil {
			t.Fatalf("UploadFile failed: %v", err)
		}

		expectedPath := filepath.Join(tmpDir, path)
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Error("File was not created in subdirectory")
		}
	})

	t.Run("upload to invalid path", func(t *testing.T) {
		invalidProvider := NewFSProvider("/invalid/path/that/does/not/exist")
		content := []byte("test content")

		_, err := invalidProvider.UploadFile(context.Background(), "test.jpg", content)
		if err == nil {
			t.Fatal("Expected error for invalid base path")
		}

		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("Expected ErrPermissionDenied, got %v", err)
		}
	})
}

func TestFSProviderGetFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fs-provider-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider := NewFSProvider(tmpDir)

	t.Run("successful get", func(t *testing.T) {
		content := []byte("test file content")
		path := "test.jpg"
		fullPath := filepath.Join(tmpDir, path)

		err := os.WriteFile(fullPath, content, 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		retrievedContent, err := provider.GetFile(context.Background(), path)
		if err != nil {
			t.Fatalf("GetFile failed: %v", err)
		}

		if string(retrievedContent) != string(content) {
			t.Errorf("Expected content '%s', got '%s'", string(content), string(retrievedContent))
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := provider.GetFile(context.Background(), "nonexistent.jpg")
		if err == nil {
			t.Fatal("Expected error for nonexistent file")
		}

		if !errors.Is(err, ErrImageNotFound) {
			t.Errorf("Expected ErrImageNotFound, got %v", err)
		}
	})

	t.Run("get file with path traversal protection", func(t *testing.T) {
		_, err := provider.GetFile(context.Background(), "../../../etc/passwd")
		if err == nil {
			t.Fatal("Expected error for path traversal attempt")
		}
	})
}

func TestFSProviderDeleteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fs-provider-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider := NewFSProvider(tmpDir)

	t.Run("successful delete", func(t *testing.T) {
		content := []byte("test file content")
		path := "test.jpg"
		fullPath := filepath.Join(tmpDir, path)

		err := os.WriteFile(fullPath, content, 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		err = provider.DeleteFile(context.Background(), path)
		if err != nil {
			t.Fatalf("DeleteFile failed: %v", err)
		}

		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Error("File should have been deleted")
		}
	})

	t.Run("delete nonexistent file", func(t *testing.T) {
		err := provider.DeleteFile(context.Background(), "nonexistent.jpg")
		if err == nil {
			t.Fatal("Expected error for nonexistent file")
		}

		if !errors.Is(err, ErrImageNotFound) {
			t.Errorf("Expected ErrImageNotFound, got %v", err)
		}
	})
}

func TestFSProviderChunkedLifecycle(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	provider := NewFSProvider(tmpDir)

	session := &ChunkSession{
		ID:            "session-1",
		Key:           "chunks/output.bin",
		TotalSize:     8,
		UploadedParts: make(map[int]ChunkPart),
	}

	if _, err := provider.InitiateChunked(ctx, session); err != nil {
		t.Fatalf("InitiateChunked failed: %v", err)
	}

	part1, err := provider.UploadChunk(ctx, session, 0, bytes.NewReader([]byte("abcd")))
	if err != nil {
		t.Fatalf("UploadChunk part1 failed: %v", err)
	}
	session.UploadedParts[0] = part1

	part2, err := provider.UploadChunk(ctx, session, 1, bytes.NewReader([]byte("efgh")))
	if err != nil {
		t.Fatalf("UploadChunk part2 failed: %v", err)
	}
	session.UploadedParts[1] = part2

	meta, err := provider.CompleteChunked(ctx, session)
	if err != nil {
		t.Fatalf("CompleteChunked failed: %v", err)
	}

	if meta.URL == "" {
		t.Fatalf("expected meta URL to be set")
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "chunks", "output.bin"))
	if err != nil {
		t.Fatalf("reading combined file failed: %v", err)
	}

	if string(content) != "abcdefgh" {
		t.Fatalf("expected combined content 'abcdefgh', got %s", string(content))
	}
}

func TestFSProviderAbortChunked(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	provider := NewFSProvider(tmpDir)

	session := &ChunkSession{
		ID:        "session-abort",
		Key:       "chunks/abort.bin",
		TotalSize: 4,
	}

	if _, err := provider.InitiateChunked(ctx, session); err != nil {
		t.Fatalf("InitiateChunked failed: %v", err)
	}

	if _, err := provider.UploadChunk(ctx, session, 0, bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("UploadChunk failed: %v", err)
	}

	if err := provider.AbortChunked(ctx, session); err != nil {
		t.Fatalf("AbortChunked failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".chunks", session.ID)); !os.IsNotExist(err) {
		t.Fatalf("expected chunk directory to be removed")
	}
}

func TestFSProviderGetPresignedURL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fs-provider-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider := NewFSProvider(tmpDir)

	t.Run("successful presigned URL", func(t *testing.T) {
		content := []byte("test file content")
		path := "test.jpg"
		fullPath := filepath.Join(tmpDir, path)

		err := os.WriteFile(fullPath, content, 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		url, err := provider.GetPresignedURL(context.Background(), path, time.Hour)
		if err != nil {
			t.Fatalf("GetPresignedURL failed: %v", err)
		}

		expectedURL := "/test.jpg"
		if url != expectedURL {
			t.Errorf("Expected URL '%s', got '%s'", expectedURL, url)
		}
	})

	t.Run("presigned URL with prefix", func(t *testing.T) {
		prefixProvider := provider.WithURLPrefix("http://example.com/files")
		content := []byte("test file content")
		path := "test.jpg"
		fullPath := filepath.Join(tmpDir, path)

		err := os.WriteFile(fullPath, content, 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		url, err := prefixProvider.GetPresignedURL(context.Background(), path, time.Hour)
		if err != nil {
			t.Fatalf("GetPresignedURL failed: %v", err)
		}

		expected := "http://example.com/files/" + path
		if url != expected {
			t.Errorf("Expected URL '%s', got '%s'", expected, url)
		}
	})

	t.Run("presigned URL for nonexistent file", func(t *testing.T) {
		_, err := provider.GetPresignedURL(context.Background(), "nonexistent.jpg", time.Hour)
		if err == nil {
			t.Fatal("Expected error for nonexistent file")
		}

		if !errors.Is(err, ErrImageNotFound) {
			t.Errorf("Expected ErrImageNotFound, got %v", err)
		}
	})
}

func TestFSProviderValidate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "fs-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		provider := NewFSProvider(tmpDir)

		err = provider.Validate(context.Background())
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
	})

	t.Run("empty base path", func(t *testing.T) {
		provider := NewFSProvider("")

		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for empty base path")
		}

		if !strings.Contains(err.Error(), "base path not configured") {
			t.Errorf("Expected error message to contain 'base path not configured', got '%s'", err.Error())
		}
	})

	t.Run("nonexistent base path", func(t *testing.T) {
		provider := NewFSProvider("/nonexistent/path")

		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for nonexistent base path")
		}

		if !strings.Contains(err.Error(), "stat base path") {
			t.Errorf("Expected error message to contain 'stat base path', got '%s'", err.Error())
		}
	})

	t.Run("base path is not directory", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "fs-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		provider := NewFSProvider(tmpFile.Name())

		err = provider.Validate(context.Background())
		if err == nil {
			t.Fatal("Expected error for file instead of directory")
		}

		if !strings.Contains(err.Error(), "base path is not a directory") {
			t.Errorf("Expected error message to contain 'base path is not a directory', got '%s'", err.Error())
		}
	})

	t.Run("read-only directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "fs-provider-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		err = os.Chmod(tmpDir, 0444)
		if err != nil {
			t.Fatalf("Failed to change permissions: %v", err)
		}

		provider := NewFSProvider(tmpDir)

		err = provider.Validate(context.Background())
		if err == nil {
			os.Chmod(tmpDir, 0755)
			t.Fatal("Expected error for read-only directory")
		}

		os.Chmod(tmpDir, 0755)

		if !strings.Contains(err.Error(), "create temp file") {
			t.Errorf("Expected error message to contain 'create temp file', got '%s'", err.Error())
		}
	})
}

func TestJoinSegments(t *testing.T) {
	tests := []struct {
		prefix   string
		path     string
		expected string
	}{
		{"http://example.com", "test.jpg", "http://example.com/test.jpg"},
		{"http://example.com/", "test.jpg", "http://example.com/test.jpg"},
		{"http://example.com", "/test.jpg", "http://example.com/test.jpg"},
		{"http://example.com/", "/test.jpg", "http://example.com/test.jpg"},
		{"", "test.jpg", "/test.jpg"},
		{"/files", "test.jpg", "/files/test.jpg"},
	}

	for _, test := range tests {
		t.Run(test.prefix+"_"+test.path, func(t *testing.T) {
			result := joinSegments(test.prefix, test.path)
			if result != test.expected {
				t.Errorf("Expected '%s', got '%s'", test.expected, result)
			}
		})
	}
}

func TestFSProviderInterface(t *testing.T) {
	var _ Uploader = &FSProvider{}
	var _ ProviderValidator = &FSProvider{}
}
