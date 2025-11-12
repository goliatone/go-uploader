package uploader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	_ Uploader        = &FSProvider{}
	_ ChunkedUploader = &FSProvider{}
	_ PresignedPoster = &FSProvider{}
)

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

func (p *FSProvider) InitiateChunked(_ context.Context, session *ChunkSession) (*ChunkSession, error) {
	if session == nil {
		return nil, fmt.Errorf("fs provider: chunk session is nil")
	}

	dir := p.chunkDir(session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("fs provider: create chunk directory: %w", err)
	}

	return session, nil
}

func (p *FSProvider) UploadChunk(_ context.Context, session *ChunkSession, index int, payload io.Reader) (ChunkPart, error) {
	if session == nil {
		return ChunkPart{}, fmt.Errorf("fs provider: chunk session is nil")
	}

	if payload == nil {
		return ChunkPart{}, fmt.Errorf("fs provider: payload reader is nil")
	}

	if index < 0 {
		return ChunkPart{}, ErrChunkPartOutOfRange
	}

	dir := p.chunkDir(session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ChunkPart{}, fmt.Errorf("fs provider: ensure chunk directory: %w", err)
	}

	chunkPath := p.chunkFilePath(session.ID, index)
	if _, err := os.Stat(chunkPath); err == nil {
		return ChunkPart{}, ErrChunkPartDuplicate
	}

	file, err := os.Create(chunkPath)
	if err != nil {
		return ChunkPart{}, fmt.Errorf("fs provider: create chunk file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, payload)
	if err != nil {
		return ChunkPart{}, fmt.Errorf("fs provider: write chunk: %w", err)
	}

	return ChunkPart{
		Index:      index,
		Size:       written,
		UploadedAt: time.Now(),
	}, nil
}

func (p *FSProvider) CompleteChunked(_ context.Context, session *ChunkSession) (*FileMeta, error) {
	if session == nil {
		return nil, fmt.Errorf("fs provider: chunk session is nil")
	}

	if len(session.UploadedParts) == 0 {
		return nil, fmt.Errorf("fs provider: no parts uploaded for session %s", session.ID)
	}

	fullPath := filepath.Join(p.base, filepath.Clean(session.Key))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, fmt.Errorf("fs provider: ensure destination dir: %w", err)
	}

	dest, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("fs provider: create destination file: %w", err)
	}
	defer dest.Close()

	indexes := make([]int, 0, len(session.UploadedParts))
	for idx := range session.UploadedParts {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	for _, idx := range indexes {
		chunkPath := p.chunkFilePath(session.ID, idx)
		if err := appendChunk(dest, chunkPath); err != nil {
			return nil, err
		}
	}

	if err := os.RemoveAll(p.chunkDir(session.ID)); err != nil {
		return nil, fmt.Errorf("fs provider: cleanup chunks: %w", err)
	}

	return &FileMeta{
		Name:         session.Key,
		OriginalName: session.Key,
		Size:         session.TotalSize,
		URL:          fullPath,
	}, nil
}

func (p *FSProvider) AbortChunked(_ context.Context, session *ChunkSession) error {
	if session == nil {
		return fmt.Errorf("fs provider: chunk session is nil")
	}

	return os.RemoveAll(p.chunkDir(session.ID))
}

func (p *FSProvider) CreatePresignedPost(context.Context, string, *Metadata) (*PresignedPost, error) {
	return nil, ErrNotImplemented
}

func joinSegments(prefix, path string) string {
	path = strings.TrimPrefix(path, "/")

	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	return prefix + path
}

func (p *FSProvider) chunkDir(sessionID string) string {
	return filepath.Join(p.base, ".chunks", sessionID)
}

func (p *FSProvider) chunkFilePath(sessionID string, index int) string {
	return filepath.Join(p.chunkDir(sessionID), fmt.Sprintf("%08d.part", index))
}

func appendChunk(dst *os.File, chunkPath string) error {
	src, err := os.Open(chunkPath)
	if err != nil {
		return fmt.Errorf("fs provider: open chunk: %w", err)
	}
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("fs provider: append chunk: %w", err)
	}

	return nil
}
