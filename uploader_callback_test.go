package uploader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"
)

func TestCallbackBestEffortHandleFile(t *testing.T) {
	ctx := context.Background()
	provider := newMemoryProvider()

	manager := NewManager()
	WithProvider(provider)(manager)

	invoked := 0
	WithOnUploadComplete(func(ctx context.Context, meta *FileMeta) error {
		invoked++
		return errors.New("boom")
	})(manager)

	header := newTestFileHeader(t, "file", "sample.png", "image/png", createTestPNG(10, 10))
	if _, err := manager.HandleFile(ctx, header, "images"); err != nil {
		t.Fatalf("expected best-effort callback to not fail upload: %v", err)
	}

	if invoked != 1 {
		t.Fatalf("expected callback invoked once, got %d", invoked)
	}
}

func TestCallbackStrictHandleFile(t *testing.T) {
	ctx := context.Background()
	provider := newMemoryProvider()

	manager := NewManager()
	WithProvider(provider)(manager)
	WithOnUploadComplete(func(ctx context.Context, meta *FileMeta) error {
		return errors.New("fail")
	})(manager)
	WithCallbackMode(CallbackModeStrict)(manager)

	header := newTestFileHeader(t, "file", "sample.png", "image/png", createTestPNG(10, 10))
	if _, err := manager.HandleFile(ctx, header, "images"); err == nil {
		t.Fatalf("expected strict callback failure to bubble up")
	}

	if len(provider.deleted) == 0 {
		t.Fatalf("expected uploaded file to be cleaned up")
	}
}

func TestCallbackTriggeredOnChunkCompletion(t *testing.T) {
	ctx := context.Background()
	provider := newMemoryProvider()

	manager := NewManager()
	WithProvider(provider)(manager)

	done := make(chan struct{}, 1)
	WithOnUploadComplete(func(ctx context.Context, meta *FileMeta) error {
		done <- struct{}{}
		return nil
	})(manager)

	session, err := manager.InitiateChunked(ctx, "chunks/file.bin", 8)
	if err != nil {
		t.Fatalf("InitiateChunked: %v", err)
	}

	if err := manager.UploadChunk(ctx, session.ID, 0, bytes.NewReader([]byte("abcd"))); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}
	if err := manager.UploadChunk(ctx, session.ID, 1, bytes.NewReader([]byte("efgh"))); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}

	if _, err := manager.CompleteChunked(ctx, session.ID); err != nil {
		t.Fatalf("CompleteChunked: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected callback after chunk completion")
	}
}

func TestAsyncCallbackExecutor(t *testing.T) {
	ctx := context.Background()
	provider := newMemoryProvider()

	manager := NewManager()
	WithProvider(provider)(manager)
	WithCallbackExecutor(NewAsyncCallbackExecutor(manager.logger))(manager)

	done := make(chan struct{})
	WithOnUploadComplete(func(ctx context.Context, meta *FileMeta) error {
		close(done)
		return nil
	})(manager)

	header := newTestFileHeader(t, "file", "sample.png", "image/png", createTestPNG(10, 10))
	if _, err := manager.HandleFile(ctx, header, "images"); err != nil {
		t.Fatalf("HandleFile: %v", err)
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("async callback not invoked")
	}
}

func TestConfirmPresignedUploadCallback(t *testing.T) {
	ctx := context.Background()
	provider := newMemoryProvider()

	manager := NewManager()
	WithProvider(provider)(manager)

	called := false
	WithOnUploadComplete(func(ctx context.Context, meta *FileMeta) error {
		called = true
		return nil
	})(manager)

	provider.files["uploads/direct.jpg"] = []byte("data")

	_, err := manager.ConfirmPresignedUpload(ctx, &PresignedUploadResult{
		Key:         "uploads/direct.jpg",
		Size:        int64(len("data")),
		ContentType: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ConfirmPresignedUpload: %v", err)
	}

	if !called {
		t.Fatalf("expected callback after presigned confirmation")
	}
}

type memoryProvider struct {
	files    map[string][]byte
	deleted  []string
	sessions map[string]*ChunkSession
}

func newMemoryProvider() *memoryProvider {
	return &memoryProvider{
		files:    make(map[string][]byte),
		sessions: make(map[string]*ChunkSession),
	}
}

func (p *memoryProvider) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	p.files[path] = append([]byte(nil), content...)
	return path, nil
}

func (p *memoryProvider) GetFile(ctx context.Context, path string) ([]byte, error) {
	if data, ok := p.files[path]; ok {
		return append([]byte(nil), data...), nil
	}
	return nil, errors.New("not found")
}

func (p *memoryProvider) DeleteFile(ctx context.Context, path string) error {
	delete(p.files, path)
	p.deleted = append(p.deleted, path)
	return nil
}

func (p *memoryProvider) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	return "mem://" + path, nil
}

func (p *memoryProvider) InitiateChunked(ctx context.Context, session *ChunkSession) (*ChunkSession, error) {
	sessionCopy := *session
	sessionCopy.UploadedParts = make(map[int]ChunkPart)
	if sessionCopy.ProviderData == nil {
		sessionCopy.ProviderData = make(map[string]any)
	}
	p.sessions[session.ID] = &sessionCopy
	return &sessionCopy, nil
}

func (p *memoryProvider) UploadChunk(ctx context.Context, session *ChunkSession, index int, payload io.Reader) (ChunkPart, error) {
	data, err := io.ReadAll(payload)
	if err != nil {
		return ChunkPart{}, err
	}
	stored := p.sessions[session.ID]
	if stored.ProviderData == nil {
		stored.ProviderData = make(map[string]any)
	}
	stored.UploadedParts[index] = ChunkPart{Index: index, Size: int64(len(data)), UploadedAt: time.Now()}
	stored.ProviderData[fmt.Sprintf("part_%d", index)] = append([]byte(nil), data...)
	return ChunkPart{Index: index, Size: int64(len(data))}, nil
}

func (p *memoryProvider) CompleteChunked(ctx context.Context, session *ChunkSession) (*FileMeta, error) {
	stored := p.sessions[session.ID]
	var keys []int
	for k := range stored.UploadedParts {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	combined := make([]byte, 0)
	for _, k := range keys {
		partKey := fmt.Sprintf("part_%d", k)
		combined = append(combined, stored.ProviderData[partKey].([]byte)...)
	}
	p.files[session.Key] = combined
	return &FileMeta{Name: session.Key, Size: int64(len(combined))}, nil
}

func (p *memoryProvider) AbortChunked(ctx context.Context, session *ChunkSession) error {
	delete(p.sessions, session.ID)
	return nil
}
