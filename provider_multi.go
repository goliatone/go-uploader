package uploader

import (
	"context"
	"fmt"
	"io"
	"time"
)

var (
	_ Uploader        = &MultiProvider{}
	_ ChunkedUploader = &MultiProvider{}
	_ PresignedPoster = &MultiProvider{}
)

type MultiProvider struct {
	logger      Logger
	local       *FSProvider
	objectStore Uploader
}

func NewMultiProvider(local *FSProvider, objectStore Uploader) *MultiProvider {
	return &MultiProvider{
		local:       local,
		logger:      &DefaultLogger{},
		objectStore: objectStore,
	}
}

func (p *MultiProvider) WithLogger(l Logger) *MultiProvider {
	p.logger = l
	return p
}

func (m *MultiProvider) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	var err error
	var url string
	if url, err = m.objectStore.UploadFile(ctx, path, content, opts...); err != nil {
		return "", err
	}

	if _, err := m.local.UploadFile(ctx, path, content, opts...); err != nil {
		return "", err
	}

	return url, nil
}

func (m *MultiProvider) GetFile(ctx context.Context, path string) ([]byte, error) {
	img, err := m.local.GetFile(ctx, path)
	if err == nil {
		return img, nil
	}
	return m.objectStore.GetFile(ctx, path)
}

func (m *MultiProvider) DeleteFile(ctx context.Context, path string) error {
	m.local.DeleteFile(ctx, path)
	return m.objectStore.DeleteFile(ctx, path)
}

func (m *MultiProvider) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	return m.objectStore.GetPresignedURL(ctx, path, expires)
}

func (m *MultiProvider) Validate(ctx context.Context) error {
	if m.local == nil {
		return fmt.Errorf("multi provider: local provider not configured")
	}

	if err := validateOptional(ctx, m.local); err != nil {
		return fmt.Errorf("multi provider: local validation failed: %w", err)
	}

	if m.objectStore == nil {
		return fmt.Errorf("multi provider: object store not configured")
	}

	if err := validateOptional(ctx, m.objectStore); err != nil {
		return fmt.Errorf("multi provider: object store validation failed: %w", err)
	}

	return nil
}

func (m *MultiProvider) InitiateChunked(ctx context.Context, session *ChunkSession) (*ChunkSession, error) {
	chunked, err := m.chunkedObjectStore()
	if err != nil {
		return nil, err
	}

	return chunked.InitiateChunked(ctx, session)
}

func (m *MultiProvider) UploadChunk(ctx context.Context, session *ChunkSession, index int, payload io.Reader) (ChunkPart, error) {
	chunked, err := m.chunkedObjectStore()
	if err != nil {
		return ChunkPart{}, err
	}

	return chunked.UploadChunk(ctx, session, index, payload)
}

func (m *MultiProvider) CompleteChunked(ctx context.Context, session *ChunkSession) (*FileMeta, error) {
	chunked, err := m.chunkedObjectStore()
	if err != nil {
		return nil, err
	}

	meta, err := chunked.CompleteChunked(ctx, session)
	if err != nil {
		return nil, err
	}

	// sync to local storage for caching
	content, err := m.objectStore.GetFile(ctx, session.Key)
	if err != nil {
		return nil, fmt.Errorf("multi provider: fetch completed file: %w", err)
	}

	if _, err := m.local.UploadFile(ctx, session.Key, content, WithContentType(meta.ContentType)); err != nil {
		return nil, fmt.Errorf("multi provider: sync to local storage: %w", err)
	}

	return meta, nil
}

func (m *MultiProvider) AbortChunked(ctx context.Context, session *ChunkSession) error {
	chunked, err := m.chunkedObjectStore()
	if err != nil {
		return err
	}

	return chunked.AbortChunked(ctx, session)
}

func (m *MultiProvider) CreatePresignedPost(ctx context.Context, key string, metadata *Metadata) (*PresignedPost, error) {
	presigner, err := m.presignedObjectStore()
	if err != nil {
		return nil, err
	}

	return presigner.CreatePresignedPost(ctx, key, metadata)
}

func validateOptional(ctx context.Context, provider Uploader) error {
	validator, ok := provider.(ProviderValidator)
	if !ok {
		return nil
	}

	return validator.Validate(ctx)
}

func (m *MultiProvider) chunkedObjectStore() (ChunkedUploader, error) {
	if m.objectStore == nil {
		return nil, fmt.Errorf("multi provider: object store not configured")
	}

	chunked, ok := m.objectStore.(ChunkedUploader)
	if !ok {
		return nil, ErrNotImplemented
	}

	if m.local == nil {
		return nil, fmt.Errorf("multi provider: local provider not configured")
	}

	return chunked, nil
}

func (m *MultiProvider) presignedObjectStore() (PresignedPoster, error) {
	if m.objectStore == nil {
		return nil, fmt.Errorf("multi provider: object store not configured")
	}

	presigner, ok := m.objectStore.(PresignedPoster)
	if !ok {
		return nil, ErrNotImplemented
	}

	return presigner, nil
}
