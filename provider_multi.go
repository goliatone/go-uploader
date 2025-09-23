package uploader

import (
	"context"
	"fmt"
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

func validateOptional(ctx context.Context, provider Uploader) error {
	validator, ok := provider.(ProviderValidator)
	if !ok {
		return nil
	}

	return validator.Validate(ctx)
}
