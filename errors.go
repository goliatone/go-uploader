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
)
