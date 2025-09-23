package uploader

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var _ Uploader = &FSProvider{}

type FSProvider struct {
	root      fs.FS
	base      string
	urlPrefix string
	logger    Logger
}

func NewFSProvider(base string) *FSProvider {
	return &FSProvider{
		root:   os.DirFS(base),
		base:   base,
		logger: &DefaultLogger{},
	}
}

func (p *FSProvider) WithLogger(l Logger) *FSProvider {
	p.logger = l
	return p
}

func (p *FSProvider) WithFS(f fs.FS) *FSProvider {
	p.root = f
	return p
}

func (p *FSProvider) WithURLPrefix(prefix string) *FSProvider {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	p.urlPrefix = prefix
	return p
}

func (p *FSProvider) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	fullPath := filepath.Join(p.base, filepath.Clean(path))
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("%w: %w", ErrPermissionDenied, err)
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return "", fmt.Errorf("%w: %s", ErrPermissionDenied, err)
	}

	return fullPath, nil
}

func (p *FSProvider) GetFile(ctx context.Context, path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	data, err := fs.ReadFile(p.root, cleanPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrImageNotFound
	}

	if errors.Is(err, fs.ErrPermission) {
		return nil, ErrPermissionDenied
	}

	if err != nil {
		return nil, fmt.Errorf("fs read: %w", err)
	}

	return data, nil
}

func (p *FSProvider) DeleteFile(ctx context.Context, path string) error {
	fullPath := filepath.Join(p.base, filepath.Clean(path))
	err := os.Remove(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return ErrImageNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return ErrPermissionDenied
	}

	if err != nil {
		return fmt.Errorf("fs read: %w", err)
	}
	return nil
}

func (p *FSProvider) GetPresignedURL(ctx context.Context, path string, _ time.Duration) (string, error) {
	if _, err := fs.Stat(p.root, filepath.Clean(path)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrImageNotFound
		}
		return "", err
	}

	return joinSegments(p.urlPrefix, path), nil
}

func (p *FSProvider) Validate(ctx context.Context) error {
	if p.base == "" {
		return fmt.Errorf("fs provider: base path not configured")
	}

	info, err := os.Stat(p.base)
	if err != nil {
		return fmt.Errorf("fs provider: stat base path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("fs provider: base path is not a directory: %s", p.base)
	}

	tmpFile, err := os.CreateTemp(p.base, ".go-uploader-*")
	if err != nil {
		return fmt.Errorf("fs provider: create temp file: %w", err)
	}

	name := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("fs provider: close temp file: %w", err)
	}

	if err := os.Remove(name); err != nil {
		return fmt.Errorf("fs provider: cleanup temp file: %w", err)
	}

	return nil
}

func joinSegments(prefix, path string) string {
	path = strings.TrimPrefix(path, "/")

	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	return prefix + path
}
