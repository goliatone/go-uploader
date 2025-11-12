package uploader

import "time"

var (
	// DefaultChunkSessionTTL is the fallback expiration applied to chunked upload sessions
	// when a custom TTL is not provided.
	DefaultChunkSessionTTL = 30 * time.Minute

	// DefaultChunkPartSize defines the default size (bytes) used for chunked uploads when
	// callers do not provide a custom size.
	DefaultChunkPartSize int64 = 5 * 1024 * 1024

	// DefaultPresignedPostTTL controls how long presigned posts remain valid when a custom TTL is not supplied.
	DefaultPresignedPostTTL = 15 * time.Minute

	// MaxPresignedPostTTL caps presigned post lifetimes to avoid long-lived public upload surfaces.
	MaxPresignedPostTTL = 24 * time.Hour

	// DefaultPresignedURLTTL determines how long confirmation URLs remain valid when returned via ConfirmPresignedUpload.
	DefaultPresignedURLTTL = 10 * time.Minute

	// DefaultPresignedMaxFileSize enforces the default max payload accepted via presigned uploads (matches validator default).
	DefaultPresignedMaxFileSize = DefaultMaxFileSize
)

// CallbackMode describes how the manager should react when post-upload callbacks fail.
type CallbackMode string

const (
	// CallbackModeStrict propagates callback errors back to the caller and should trigger cleanup.
	CallbackModeStrict CallbackMode = "strict"
	// CallbackModeBestEffort logs callback failures but still reports success to the caller.
	CallbackModeBestEffort CallbackMode = "best_effort"
)
