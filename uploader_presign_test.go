package uploader

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestManagerCreatePresignedPost(t *testing.T) {
	ctx := context.Background()

	post := &PresignedPost{
		URL:    "https://example.com/upload",
		Method: "POST",
		Fields: map[string]string{"key": "uploads/file.jpg"},
		Expiry: time.Now().Add(10 * time.Minute),
	}

	provider := &stubPresignProvider{post: post}
	manager := NewManager()
	WithProvider(provider)(manager)

	result, err := manager.CreatePresignedPost(ctx, "uploads/file.jpg", WithContentType("image/jpeg"))
	if err != nil {
		t.Fatalf("CreatePresignedPost returned error: %v", err)
	}

	if result.URL != post.URL {
		t.Fatalf("expected URL %s, got %s", post.URL, result.URL)
	}

	if provider.meta == nil || provider.meta.ContentType != "image/jpeg" {
		t.Fatalf("expected provider to receive metadata with content type")
	}
}

func TestManagerCreatePresignedPostValidatesContentType(t *testing.T) {
	ctx := context.Background()
	manager := NewManager()
	WithProvider(&stubUploader{})(manager)

	_, err := manager.CreatePresignedPost(ctx, "uploads/file.jpg")
	if err == nil {
		t.Fatalf("expected error when content type is missing")
	}
}

func TestManagerCreatePresignedPostProviderRequirement(t *testing.T) {
	ctx := context.Background()
	manager := NewManager()
	WithProvider(&stubUploader{})(manager)

	_, err := manager.CreatePresignedPost(ctx, "uploads/file.jpg", WithContentType("image/jpeg"))
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestManagerConfirmPresignedUpload(t *testing.T) {
	ctx := context.Background()
	provider := &stubPresignProvider{
		presignedURL: "https://example.com/asset",
	}

	manager := NewManager()
	WithProvider(provider)(manager)

	meta, err := manager.ConfirmPresignedUpload(ctx, &PresignedUploadResult{
		Key:         "uploads/file.jpg",
		Size:        1024,
		ContentType: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ConfirmPresignedUpload returned error: %v", err)
	}

	if meta.URL != provider.presignedURL {
		t.Fatalf("expected URL %s, got %s", provider.presignedURL, meta.URL)
	}
}

type stubPresignProvider struct {
	post         *PresignedPost
	meta         *Metadata
	presignedURL string
}

func (s *stubPresignProvider) UploadFile(context.Context, string, []byte, ...UploadOption) (string, error) {
	return "", nil
}

func (s *stubPresignProvider) GetFile(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (s *stubPresignProvider) DeleteFile(context.Context, string) error { return nil }

func (s *stubPresignProvider) GetPresignedURL(context.Context, string, time.Duration) (string, error) {
	if s.presignedURL == "" {
		return "https://example.com/temp", nil
	}
	return s.presignedURL, nil
}

func (s *stubPresignProvider) CreatePresignedPost(_ context.Context, _ string, metadata *Metadata) (*PresignedPost, error) {
	s.meta = metadata
	if s.post != nil {
		return s.post, nil
	}
	return &PresignedPost{
		URL:    "https://example.com/upload",
		Method: "POST",
		Fields: map[string]string{},
		Expiry: time.Now().Add(10 * time.Minute),
	}, nil
}
