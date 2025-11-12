package uploader

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleImageWithThumbnails(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	provider := NewFSProvider(dir)
	manager := NewManager()
	WithProvider(provider)(manager)

	fileBytes := createTestPNG(20, 20)
	fh := newTestFileHeader(t, "file", "sample.png", "image/png", fileBytes)

	sizes := []ThumbnailSize{{Name: "small", Width: 8, Height: 8, Fit: "cover"}}
	meta, err := manager.HandleImageWithThumbnails(ctx, fh, "images", sizes)
	if err != nil {
		t.Fatalf("HandleImageWithThumbnails returned error: %v", err)
	}

	if meta == nil || meta.FileMeta == nil {
		t.Fatalf("expected image meta")
	}

	if len(meta.Thumbnails) != 1 {
		t.Fatalf("expected 1 thumbnail, got %d", len(meta.Thumbnails))
	}

	thumb := meta.Thumbnails["small"]
	if thumb == nil {
		t.Fatalf("thumbnail missing")
	}

	if thumb.Name == meta.Name {
		t.Fatalf("thumbnail should have unique name")
	}
}

func TestHandleImageWithThumbnailsValidation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	provider := NewFSProvider(dir)
	manager := NewManager()
	WithProvider(provider)(manager)

	fh := newTestFileHeader(t, "file", "sample.png", "image/png", createTestPNG(10, 10))

	_, err := manager.HandleImageWithThumbnails(ctx, fh, "images", nil)
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func newTestFileHeader(t *testing.T, field, filename, contentType string, data []byte) *multipart.FileHeader {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write data: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(int64(len(buf.Bytes()))); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}

	return req.MultipartForm.File[field][0]
}
