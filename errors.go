package uploader

import (
	gerrors "github.com/goliatone/go-errors"
)

var (
	ErrImageNotFound = gerrors.New("image not found", gerrors.CategoryNotFound).
				WithCode(404).
				WithTextCode("IMAGE_NOT_FOUND")

	ErrPermissionDenied = gerrors.New("permission denied", gerrors.CategoryAuthz).
				WithCode(403).
				WithTextCode("PERMISSION_DENIED")

	ErrInvalidPath = gerrors.New("invalid path", gerrors.CategoryBadInput).
			WithCode(400).
			WithTextCode("INVALID_PATH")

	ErrProviderNotConfigured = gerrors.New("provider not configured", gerrors.CategoryInternal).
					WithCode(500).
					WithTextCode("PROVIDER_NOT_CONFIGURED")

	ErrNotImplemented = gerrors.New("feature not implemented", gerrors.CategoryInternal).
				WithCode(501).
				WithTextCode("NOT_IMPLEMENTED")

	ErrChunkSessionNotFound = gerrors.New("chunk session not found", gerrors.CategoryNotFound).
				WithCode(404).
				WithTextCode("CHUNK_SESSION_NOT_FOUND")

	ErrChunkSessionExists = gerrors.New("chunk session already exists", gerrors.CategoryConflict).
				WithCode(409).
				WithTextCode("CHUNK_SESSION_EXISTS")

	ErrChunkSessionClosed = gerrors.New("chunk session is no longer active", gerrors.CategoryConflict).
				WithCode(409).
				WithTextCode("CHUNK_SESSION_CLOSED")

	ErrChunkPartOutOfRange = gerrors.New("chunk part index is out of range", gerrors.CategoryBadInput).
				WithCode(400).
				WithTextCode("CHUNK_PART_OUT_OF_RANGE")

	ErrChunkPartDuplicate = gerrors.New("chunk part already uploaded", gerrors.CategoryConflict).
				WithCode(409).
				WithTextCode("CHUNK_PART_DUPLICATE")
)
