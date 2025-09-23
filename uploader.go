package uploader

import (
	"context"
	"io"
	"mime/multipart"
	"time"

	gerrors "github.com/goliatone/go-errors"
)

type Metadata struct {
	ContentType  string
	CacheControl string
	Public       bool
	TTL          time.Duration
}

type UploadOption func(*Metadata)

func WithContentType(t string) UploadOption {
	return func(m *Metadata) { m.ContentType = t }
}

func WithCacheControl(c string) UploadOption {
	return func(m *Metadata) { m.CacheControl = c }
}

func WithPublicAccess(a bool) UploadOption {
	return func(m *Metadata) { m.Public = a }
}

func WithTTL(ttl time.Duration) UploadOption {
	return func(m *Metadata) { m.TTL = ttl }
}

type Uploader interface {
	UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error)
	GetFile(ctx context.Context, path string) ([]byte, error)
	DeleteFile(ctx context.Context, path string) error
	GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error)
}

type ProviderValidator interface {
	Validate(context.Context) error
}

var _ Uploader = &Manager{}

type Manager struct {
	logger      Logger
	provider    Uploader
	validator   *Validator
	providerErr error
	validated   bool
	validateCtx context.Context
}

type Option func(m *Manager)

func WithLogger(l Logger) Option {
	return func(m *Manager) {
		m.logger = l
	}
}

func WithProvider(p Uploader) Option {
	return func(m *Manager) {
		m.provider = p
		m.validated = false
		m.providerErr = nil

		ctx := m.validateCtx
		if ctx == nil {
			ctx = context.Background()
		}

		if err := m.validateProvider(ctx); err != nil {
			m.providerErr = err
			return
		}

		m.validated = true
	}
}

func WithValidator(v *Validator) Option {
	return func(m *Manager) {
		m.validator = v
	}
}

func WithProviderValidationContext(ctx context.Context) Option {
	return func(m *Manager) {
		m.validateCtx = ctx
	}
}

func NewManager(opts ...Option) *Manager {
	m := &Manager{
		logger:      &DefaultLogger{},
		validator:   NewValidator(),
		validateCtx: context.Background(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

type FileMeta struct {
	Content      []byte `json:"content"`
	ContentType  string `json:"content_type"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	Size         int64  `json:"size"`
	URL          string `json:"url"`
}

func (m *Manager) HandleFile(ctx context.Context, file *multipart.FileHeader, path string) (*FileMeta, error) {
	if file == nil {
		return nil, gerrors.New("file not found", gerrors.CategoryNotFound).
			WithCode(404).
			WithTextCode("FILE_NOT_FOUND").
			WithMetadata(map[string]any{
				"function": "HandleFile",
			})
	}

	if err := m.validator.ValidateFile(file); err != nil {
		return nil, err
	}

	fileBuff, err := file.Open()
	defer func(fb multipart.File) {
		_ = fb.Close()
	}(fileBuff)

	if err != nil {
		return nil, err
	}

	var url string
	var name string
	var content []byte
	contentType := file.Header["Content-Type"][0]

	if content, err = io.ReadAll(fileBuff); err != nil {
		return nil, err
	}

	if err := m.validator.ValidateFileContent(content); err != nil {
		return nil, err
	}

	if name, err = m.validator.RandomName(file, path); err != nil {
		return nil, err
	}

	if url, err = m.UploadFile(ctx, name, content, WithContentType(contentType)); err != nil {
		return nil, err
	}

	meta := &FileMeta{
		Content:      content,
		ContentType:  contentType,
		Name:         name,
		OriginalName: file.Filename,
		Size:         file.Size,
		URL:          url,
	}

	return meta, nil
}

func (m *Manager) UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error) {
	if err := m.ensureProvider(ctx); err != nil {
		return "", err
	}

	return m.provider.UploadFile(ctx, path, content, opts...)
}

func (m *Manager) GetFile(ctx context.Context, path string) ([]byte, error) {
	if err := m.ensureProvider(ctx); err != nil {
		return nil, err
	}

	return m.provider.GetFile(ctx, path)
}

func (m *Manager) DeleteFile(ctx context.Context, path string) error {
	if err := m.ensureProvider(ctx); err != nil {
		return err
	}

	return m.provider.DeleteFile(ctx, path)
}

func (m *Manager) GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error) {
	if err := m.ensureProvider(ctx); err != nil {
		return "", err
	}

	return m.provider.GetPresignedURL(ctx, path, expires)
}

func (m *Manager) ensureProvider(ctx context.Context) error {
	if m.provider == nil {
		return ErrProviderNotConfigured
	}

	if m.providerErr != nil {
		return m.providerErr
	}

	if m.validated {
		return nil
	}

	if err := m.validateProvider(ctx); err != nil {
		m.providerErr = err
		return err
	}

	m.validated = true
	return nil
}

func (m *Manager) validateProvider(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	validator, ok := m.provider.(ProviderValidator)
	if !ok {
		return nil
	}

	return validator.Validate(ctx)
}

func (m *Manager) ValidateProvider(ctx context.Context) error {
	if m.provider == nil {
		return ErrProviderNotConfigured
	}

	if err := m.validateProvider(ctx); err != nil {
		m.providerErr = err
		m.validated = false
		return err
	}

	m.providerErr = nil
	m.validated = true
	return nil
}
