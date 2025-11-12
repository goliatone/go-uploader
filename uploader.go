package uploader

import (
	"context"
	"io"
	"mime/multipart"
	"strings"
	"time"

	gerrors "github.com/goliatone/go-errors"
	"github.com/google/uuid"
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

type ChunkedUploader interface {
	InitiateChunked(ctx context.Context, session *ChunkSession) (*ChunkSession, error)
	UploadChunk(ctx context.Context, session *ChunkSession, index int, payload io.Reader) (ChunkPart, error)
	CompleteChunked(ctx context.Context, session *ChunkSession) (*FileMeta, error)
	AbortChunked(ctx context.Context, session *ChunkSession) error
}

type PresignedPoster interface {
	CreatePresignedPost(ctx context.Context, key string, metadata *Metadata) (*PresignedPost, error)
}

type ImageProcessor interface {
	Generate(ctx context.Context, source []byte, size ThumbnailSize, contentType string) ([]byte, string, error)
}

var _ Uploader = &Manager{}

type Manager struct {
	logger         Logger
	provider       Uploader
	validator      *Validator
	chunkStore     *ChunkSessionStore
	chunkPartSize  int64
	imageProcessor ImageProcessor
	providerErr    error
	validated      bool
	validateCtx    context.Context
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

func WithChunkSessionStore(store *ChunkSessionStore) Option {
	return func(m *Manager) {
		if store != nil {
			m.chunkStore = store
		}
	}
}

func WithChunkPartSize(size int64) Option {
	return func(m *Manager) {
		if size > 0 {
			m.chunkPartSize = size
		}
	}
}

func WithImageProcessor(processor ImageProcessor) Option {
	return func(m *Manager) {
		if processor != nil {
			m.imageProcessor = processor
		}
	}
}

func NewManager(opts ...Option) *Manager {
	m := &Manager{
		logger:         &DefaultLogger{},
		validator:      NewValidator(),
		validateCtx:    context.Background(),
		chunkStore:     NewChunkSessionStore(DefaultChunkSessionTTL),
		chunkPartSize:  DefaultChunkPartSize,
		imageProcessor: NewLocalImageProcessor(),
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

type ImageMeta struct {
	*FileMeta
	Thumbnails map[string]*FileMeta `json:"thumbnails"`
}

type PresignedPost struct {
	URL    string            `json:"url"`
	Method string            `json:"method"`
	Fields map[string]string `json:"fields"`
	Expiry time.Time         `json:"expiry"`
}

type PresignedUploadResult struct {
	Key          string
	OriginalName string
	Size         int64
	ContentType  string
	Metadata     map[string]string
}

func (m *Manager) InitiateChunked(ctx context.Context, key string, totalSize int64, opts ...UploadOption) (*ChunkSession, error) {
	if key == "" {
		return nil, ErrInvalidPath
	}

	if totalSize <= 0 {
		return nil, gerrors.NewValidation("chunked upload initialization failed",
			gerrors.FieldError{
				Field:   "total_size",
				Message: "must be greater than zero",
				Value:   totalSize,
			},
		).WithCode(400).WithTextCode("INVALID_CHUNK_TOTAL_SIZE")
	}

	if err := m.ensureProvider(ctx); err != nil {
		return nil, err
	}

	chunkProvider, err := m.chunkedProvider()
	if err != nil {
		return nil, err
	}

	meta := &Metadata{}
	for _, opt := range opts {
		opt(meta)
	}

	session := &ChunkSession{
		ID:        uuid.NewString(),
		Key:       key,
		TotalSize: totalSize,
		PartSize:  m.chunkPartSize,
		Metadata:  meta,
	}

	if session.ProviderData == nil {
		session.ProviderData = make(map[string]any)
	}

	if _, err := chunkProvider.InitiateChunked(ctx, session); err != nil {
		return nil, err
	}

	stored, err := m.ensureChunkStore().Create(session)
	if err != nil {
		return nil, err
	}

	return stored, nil
}

func (m *Manager) UploadChunk(ctx context.Context, sessionID string, index int, payload io.Reader) error {
	if index < 0 {
		return ErrChunkPartOutOfRange
	}

	if payload == nil {
		return gerrors.NewValidation("chunk upload failed",
			gerrors.FieldError{
				Field:   "payload",
				Message: "payload reader cannot be nil",
			},
		)
	}

	if err := m.ensureProvider(ctx); err != nil {
		return err
	}

	chunkProvider, err := m.chunkedProvider()
	if err != nil {
		return err
	}

	session, err := m.getChunkSession(sessionID)
	if err != nil {
		return err
	}

	part, err := chunkProvider.UploadChunk(ctx, session, index, payload)
	if err != nil {
		return err
	}

	if part.Index != index {
		part.Index = index
	}

	_, err = m.ensureChunkStore().AddPart(sessionID, part)
	return err
}

func (m *Manager) CompleteChunked(ctx context.Context, sessionID string) (*FileMeta, error) {
	if err := m.ensureProvider(ctx); err != nil {
		return nil, err
	}

	chunkProvider, err := m.chunkedProvider()
	if err != nil {
		return nil, err
	}

	session, err := m.getChunkSession(sessionID)
	if err != nil {
		return nil, err
	}

	meta, err := chunkProvider.CompleteChunked(ctx, session)
	if err != nil {
		return nil, err
	}

	if _, err := m.ensureChunkStore().MarkCompleted(sessionID); err != nil {
		return nil, err
	}

	m.ensureChunkStore().Delete(sessionID)
	return meta, nil
}

func (m *Manager) AbortChunked(ctx context.Context, sessionID string) error {
	if err := m.ensureProvider(ctx); err != nil {
		return err
	}

	chunkProvider, err := m.chunkedProvider()
	if err != nil {
		return err
	}

	session, err := m.getChunkSession(sessionID)
	if err != nil {
		return err
	}

	if err := chunkProvider.AbortChunked(ctx, session); err != nil {
		return err
	}

	if _, err := m.ensureChunkStore().MarkAborted(sessionID); err != nil {
		return err
	}

	m.ensureChunkStore().Delete(sessionID)
	return nil
}

func (m *Manager) CreatePresignedPost(ctx context.Context, key string, opts ...UploadOption) (*PresignedPost, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}

	if err := m.ensureProvider(ctx); err != nil {
		return nil, err
	}

	presigner, err := m.presignedProvider()
	if err != nil {
		return nil, err
	}

	meta := &Metadata{}
	for _, opt := range opts {
		opt(meta)
	}

	if meta.ContentType == "" {
		return nil, gerrors.NewValidation("presigned post validation failed",
			gerrors.FieldError{
				Field:   "content_type",
				Message: "content type is required",
			},
		)
	}

	if !m.validator.IsAllowedMimeType(meta.ContentType) {
		return nil, gerrors.NewValidation("presigned post validation failed",
			gerrors.FieldError{
				Field:   "content_type",
				Message: "content type not allowed",
				Value:   meta.ContentType,
			},
		)
	}

	ttl := meta.TTL
	if ttl <= 0 {
		ttl = DefaultPresignedPostTTL
	}

	if ttl > MaxPresignedPostTTL {
		return nil, gerrors.NewValidation("presigned post validation failed",
			gerrors.FieldError{
				Field:   "ttl",
				Message: "requested ttl exceeds maximum",
				Value:   ttl,
			},
		)
	}

	meta.TTL = ttl
	return presigner.CreatePresignedPost(ctx, key, meta)
}

func (m *Manager) ConfirmPresignedUpload(ctx context.Context, result *PresignedUploadResult) (*FileMeta, error) {
	if result == nil {
		return nil, gerrors.NewValidation("presigned upload confirmation failed",
			gerrors.FieldError{
				Field:   "result",
				Message: "result cannot be nil",
			},
		)
	}

	if err := validateObjectKey(result.Key); err != nil {
		return nil, err
	}

	if result.ContentType != "" && !m.validator.IsAllowedMimeType(result.ContentType) {
		return nil, gerrors.NewValidation("presigned upload confirmation failed",
			gerrors.FieldError{
				Field:   "content_type",
				Message: "content type not allowed",
				Value:   result.ContentType,
			},
		)
	}

	if result.Size < 0 || (result.Size > 0 && result.Size > m.validator.MaxFileSize()) {
		return nil, gerrors.NewValidation("presigned upload confirmation failed",
			gerrors.FieldError{
				Field:   "size",
				Message: "file size exceeds maximum allowed",
				Value:   result.Size,
			},
		)
	}

	if err := m.ensureProvider(ctx); err != nil {
		return nil, err
	}

	url, err := m.provider.GetPresignedURL(ctx, result.Key, DefaultPresignedURLTTL)
	if err != nil {
		return nil, err
	}

	meta := &FileMeta{
		Name:         result.Key,
		OriginalName: result.OriginalName,
		Size:         result.Size,
		ContentType:  result.ContentType,
		URL:          url,
	}

	return meta, nil
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

func (m *Manager) HandleImageWithThumbnails(ctx context.Context, file *multipart.FileHeader, path string, sizes []ThumbnailSize) (*ImageMeta, error) {
	if err := ValidateThumbnailSizes(sizes); err != nil {
		return nil, err
	}

	baseMeta, err := m.HandleFile(ctx, file, path)
	if err != nil {
		return nil, err
	}

	if baseMeta.Content == nil {
		return nil, fmt.Errorf("image meta content missing")
	}

	processor := m.ensureImageProcessor()
	thumbnails := make(map[string]*FileMeta, len(sizes))

	for _, size := range sizes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		thumbBytes, thumbContentType, err := processor.Generate(ctx, baseMeta.Content, size, baseMeta.ContentType)
		if err != nil {
			return nil, err
		}

		thumbName := buildThumbnailKey(baseMeta.Name, size.Name)
		thumbURL, err := m.UploadFile(ctx, thumbName, thumbBytes, WithContentType(thumbContentType))
		if err != nil {
			return nil, err
		}

		thumbnails[size.Name] = &FileMeta{
			ContentType:  thumbContentType,
			Name:         thumbName,
			OriginalName: fmt.Sprintf("%s__%s", baseMeta.OriginalName, size.Name),
			Size:         int64(len(thumbBytes)),
			URL:          thumbURL,
		}
	}

	return &ImageMeta{
		FileMeta:   baseMeta,
		Thumbnails: thumbnails,
	}, nil
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

func (m *Manager) chunkedProvider() (ChunkedUploader, error) {
	provider, ok := m.provider.(ChunkedUploader)
	if !ok {
		return nil, ErrNotImplemented
	}
	return provider, nil
}

func (m *Manager) ensureChunkStore() *ChunkSessionStore {
	if m.chunkStore == nil {
		m.chunkStore = NewChunkSessionStore(DefaultChunkSessionTTL)
	}
	return m.chunkStore
}

func (m *Manager) getChunkSession(id string) (*ChunkSession, error) {
	if id == "" {
		return nil, ErrChunkSessionNotFound
	}

	session, ok := m.ensureChunkStore().Get(id)
	if !ok {
		return nil, ErrChunkSessionNotFound
	}

	return session, nil
}

func (m *Manager) presignedProvider() (PresignedPoster, error) {
	if presigner, ok := m.provider.(PresignedPoster); ok {
		return presigner, nil
	}
	return nil, ErrNotImplemented
}

func validateObjectKey(key string) error {
	if key == "" {
		return ErrInvalidPath
	}

	if strings.Contains(key, "..") {
		return ErrInvalidPath
	}

	if strings.HasPrefix(key, "/") {
		return ErrInvalidPath
	}

	return nil
}

func (m *Manager) ensureImageProcessor() ImageProcessor {
	if m.imageProcessor == nil {
		m.imageProcessor = NewLocalImageProcessor()
	}
	return m.imageProcessor
}

func buildThumbnailKey(name, variant string) string {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = name
	}
	return fmt.Sprintf("%s__%s%s", base, variant, ext)
}
