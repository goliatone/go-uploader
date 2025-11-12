package uploader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestManagerChunkedLifecycle(t *testing.T) {
	ctx := context.Background()
	provider := newMockChunkUploader()

	manager := NewManager()
	WithProvider(provider)(manager)

	data := []byte("hello world from chunk uploads")

	session, err := manager.InitiateChunked(ctx, "assets/chunk.txt", int64(len(data)))
	if err != nil {
		t.Fatalf("InitiateChunked returned error: %v", err)
	}

	chunkSize := 5
	for idx := 0; idx*chunkSize < len(data); idx++ {
		start := idx * chunkSize
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}

		if err := manager.UploadChunk(ctx, session.ID, idx, bytes.NewReader(data[start:end])); err != nil {
			t.Fatalf("UploadChunk failed: %v", err)
		}
	}

	meta, err := manager.CompleteChunked(ctx, session.ID)
	if err != nil {
		t.Fatalf("CompleteChunked failed: %v", err)
	}

	if meta.Name != "assets/chunk.txt" {
		t.Fatalf("unexpected meta name: %s", meta.Name)
	}

	if got := provider.getFile("assets/chunk.txt"); !bytes.Equal(got, data) {
		t.Fatalf("expected stored data to equal original payload")
	}
}

func TestManagerChunkedAbort(t *testing.T) {
	ctx := context.Background()
	provider := newMockChunkUploader()

	manager := NewManager()
	WithProvider(provider)(manager)

	session, err := manager.InitiateChunked(ctx, "abort.bin", 10)
	if err != nil {
		t.Fatalf("InitiateChunked failed: %v", err)
	}

	if err := manager.UploadChunk(ctx, session.ID, 0, bytes.NewReader([]byte("12345"))); err != nil {
		t.Fatalf("UploadChunk failed: %v", err)
	}

	if err := manager.AbortChunked(ctx, session.ID); err != nil {
		t.Fatalf("AbortChunked failed: %v", err)
	}

	if !provider.isAborted(session.ID) {
		t.Fatalf("expected provider to record abort")
	}
}

func TestManagerChunkedRequiresProviderSupport(t *testing.T) {
	ctx := context.Background()
	manager := NewManager()
	WithProvider(&stubUploader{})(manager)

	_, err := manager.InitiateChunked(ctx, "file.bin", 100)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

type stubUploader struct{}

func (s *stubUploader) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	return "", nil
}
func (s *stubUploader) GetFile(ctx context.Context, path string) ([]byte, error) { return nil, nil }
func (s *stubUploader) DeleteFile(ctx context.Context, path string) error        { return nil }
func (s *stubUploader) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	return "", nil
}

type mockChunkUploader struct {
	files    map[string][]byte
	sessions map[string]*ChunkSession
	aborted  map[string]bool
}

func newMockChunkUploader() *mockChunkUploader {
	return &mockChunkUploader{
		files:    make(map[string][]byte),
		sessions: make(map[string]*ChunkSession),
		aborted:  make(map[string]bool),
	}
}

func (m *mockChunkUploader) UploadFile(context.Context, string, []byte, ...UploadOption) (string, error) {
	return "", nil
}
func (m *mockChunkUploader) GetFile(context.Context, string) ([]byte, error) { return nil, nil }
func (m *mockChunkUploader) DeleteFile(context.Context, string) error        { return nil }
func (m *mockChunkUploader) GetPresignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (m *mockChunkUploader) InitiateChunked(_ context.Context, session *ChunkSession) (*ChunkSession, error) {
	m.sessions[session.ID] = cloneChunkSession(session)
	return session, nil
}

func (m *mockChunkUploader) UploadChunk(_ context.Context, session *ChunkSession, index int, payload io.Reader) (ChunkPart, error) {
	data, err := io.ReadAll(payload)
	if err != nil {
		return ChunkPart{}, err
	}

	stored := m.sessions[session.ID]
	if stored == nil {
		stored = cloneChunkSession(session)
		m.sessions[session.ID] = stored
	}

	if stored.UploadedParts == nil {
		stored.UploadedParts = make(map[int]ChunkPart)
	}
	if stored.ProviderData == nil {
		stored.ProviderData = make(map[string]any)
	}

	stored.UploadedParts[index] = ChunkPart{
		Index: index,
		Size:  int64(len(data)),
		ETag:  fmt.Sprintf("etag-%d", index),
	}
	stored.ProviderData["part_"+strconv.Itoa(index)] = data
	return ChunkPart{
		Index: index,
		Size:  int64(len(data)),
		ETag:  fmt.Sprintf("etag-%d", index),
	}, nil
}

func (m *mockChunkUploader) CompleteChunked(_ context.Context, session *ChunkSession) (*FileMeta, error) {
	stored := m.sessions[session.ID]
	if stored == nil {
		return nil, fmt.Errorf("session not found")
	}

	var order []int
	for idx := range stored.UploadedParts {
		order = append(order, idx)
	}
	sort.Ints(order)

	var combined []byte
	for _, idx := range order {
		key := "part_" + strconv.Itoa(idx)
		raw, ok := stored.ProviderData[key].([]byte)
		if !ok {
			return nil, fmt.Errorf("chunk data missing for index %d", idx)
		}
		combined = append(combined, raw...)
	}

	m.files[session.Key] = combined

	return &FileMeta{
		Name: session.Key,
		Size: int64(len(combined)),
	}, nil
}

func (m *mockChunkUploader) AbortChunked(_ context.Context, session *ChunkSession) error {
	m.aborted[session.ID] = true
	return nil
}

func (m *mockChunkUploader) getFile(key string) []byte {
	return m.files[key]
}

func (m *mockChunkUploader) isAborted(id string) bool {
	return m.aborted[id]
}
