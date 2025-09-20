package uploader

import (
	"context"
	"time"
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
