package uploader

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gerrors "github.com/goliatone/go-errors"
)

var (
	DefaultMaxFileSize  int64 = 25 * 1024 * 1024
	AllowedImageFormats       = map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
		".bmp":  true,
		".tiff": true,
		".svg":  true,
	}
	AllowedImageMimeTypes = map[string]bool{
		"image/jpeg":    true,
		"image/png":     true,
		"image/gif":     true,
		"image/webp":    true,
		"image/bmp":     true,
		"image/tiff":    true,
		"image/svg+xml": true,
		"image/pdf":     true,
	}
)

func getAllowedMsg(options map[string]bool) string {
	out := []string{}
	for k, v := range options {
		if v {
			out = append(out, k)
		}
	}
	return strings.Join(out, ",")
}

type Validator struct {
	maxFileSize         int64
	allowedMimeTypes    map[string]bool
	allowedImageFormats map[string]bool
}

type ValidatorOption func(*Validator)

func WithUploadMaxFileSize(size int64) ValidatorOption {
	return func(uv *Validator) {
		uv.maxFileSize = size
	}
}

func WithAllowedMimeTypes(types map[string]bool) ValidatorOption {
	return func(uv *Validator) {
		uv.allowedMimeTypes = types
	}
}

func WithAllowedImageFormats(formats map[string]bool) ValidatorOption {
	return func(uv *Validator) {
		uv.allowedImageFormats = formats
	}
}

func NewValidator(opts ...ValidatorOption) *Validator {
	u := &Validator{
		maxFileSize:         DefaultMaxFileSize,
		allowedMimeTypes:    AllowedImageMimeTypes,
		allowedImageFormats: AllowedImageFormats,
	}

	for _, opt := range opts {
		opt(u)
	}

	return u
}

func (u *Validator) ValidateFile(file *multipart.FileHeader) error {

	if file.Size > u.maxFileSize {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_size",
				Message: fmt.Sprintf("file too large, max: %d bytes", u.maxFileSize),
				Value:   file.Size,
			},
		).WithCode(400).WithTextCode("FILE_TOO_LARGE").
			WithMetadata(map[string]any{
				"filename":     file.Filename,
				"file_size":    file.Size,
				"max_size":     u.maxFileSize,
				"content_type": file.Header.Get("Content-Type"),
			})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !u.allowedImageFormats[ext] {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_format",
				Message: fmt.Sprintf("invalid format, allowed: %s", getAllowedMsg(u.allowedImageFormats)),
				Value:   ext,
			},
		).WithCode(400).WithTextCode("INVALID_FILE_FORMAT").
			WithMetadata(map[string]any{
				"filename":        file.Filename,
				"file_extension":  ext,
				"allowed_formats": getAllowedMsg(u.allowedImageFormats),
			})
	}

	if !u.allowedMimeTypes[file.Header.Get("Content-Type")] {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "content_type",
				Message: fmt.Sprintf("invalid mime type, allowed: %s", getAllowedMsg(u.allowedMimeTypes)),
				Value:   file.Header.Get("Content-Type"),
			},
		).WithCode(400).WithTextCode("INVALID_MIME_TYPE").
			WithMetadata(map[string]any{
				"filename":          file.Filename,
				"content_type":      file.Header.Get("Content-Type"),
				"allowed_types":     getAllowedMsg(u.allowedMimeTypes),
			})
	}

	return nil
}

func (u *Validator) ValidateFileContent(content []byte) error {
	if len(content) > int(u.maxFileSize) {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_size",
				Message: fmt.Sprintf("file too large, max: %d bytes", u.maxFileSize),
				Value:   len(content),
			},
		).WithCode(400).WithTextCode("FILE_TOO_LARGE")
	}

	if !isValidFileContent(content) {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_content",
				Message: "invalid file content",
				Value:   "binary_data",
			},
		).WithCode(400).WithTextCode("INVALID_FILE_CONTENT")
	}

	return nil
}

func (u *Validator) RandomName(file *multipart.FileHeader, paths ...string) (string, error) {
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		return "", gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_extension",
				Message: "file extension not found",
				Value:   file.Filename,
			},
		).WithCode(400).WithTextCode("FILE_EXTENSION_NOT_FOUND")
	}

	randomName := strconv.FormatInt(time.Now().UnixMicro(), 10)
	imageName := randomName + ext
	if len(paths) > 0 && paths[0] != "" {
		return paths[0] + "/" + imageName, nil
	}

	return imageName, nil
}

func ValidateFile(file *multipart.FileHeader) error {
	max := DefaultMaxFileSize
	if file.Size > max {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_size",
				Message: fmt.Sprintf("file too large, max: %d bytes", max),
				Value:   file.Size,
			},
		).WithCode(400).WithTextCode("FILE_TOO_LARGE")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedImageFormats[ext] {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_format",
				Message: fmt.Sprintf("invalid format, allowed: %s", getAllowedMsg(AllowedImageFormats)),
				Value:   ext,
			},
		).WithCode(400).WithTextCode("INVALID_FILE_FORMAT")
	}

	if !AllowedImageMimeTypes[file.Header.Get("Content-Type")] {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "content_type",
				Message: fmt.Sprintf("invalid mime type, allowed: %s", getAllowedMsg(AllowedImageMimeTypes)),
				Value:   file.Header.Get("Content-Type"),
			},
		).WithCode(400).WithTextCode("INVALID_MIME_TYPE")
	}

	return nil
}

func ValidateFileContent(content []byte) error {
	max := DefaultMaxFileSize
	if len(content) > int(max) {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_size",
				Message: fmt.Sprintf("file too large, max: %d bytes", max),
				Value:   len(content),
			},
		).WithCode(400).WithTextCode("FILE_TOO_LARGE")
	}

	if !isValidFileContent(content) {
		return gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_content",
				Message: "invalid file content",
				Value:   "binary_data",
			},
		).WithCode(400).WithTextCode("INVALID_FILE_CONTENT")
	}

	return nil
}

var magicNumbers = map[string][]byte{
	"bmp":  {0x42, 0x4D},
	"gif":  {0x47, 0x49, 0x46, 0x38},
	"png":  {0x89, 0x50, 0x4E, 0x47},
	"jpeg": {0xFF, 0xD8, 0xFF},
	"webp": {0x52, 0x49, 0x46, 0x46},
}

func isValidFileContent(content []byte) bool {
	// we need to be able to read the magic numbs from header
	if len(content) < 4 {
		return false
	}

	for _, m := range magicNumbers {
		if len(content) >= len(m) && compareBytes(content[:len(m)], m) {
			return true
		}
	}
	return false
}

func compareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func RandomName(file *multipart.FileHeader, paths ...string) (string, error) {
	ext := filepath.Ext(file.Filename)

	if ext == "" {
		return "", gerrors.NewValidation("file validation failed",
			gerrors.FieldError{
				Field:   "file_extension",
				Message: "file extension not found",
				Value:   file.Filename,
			},
		).WithCode(400).WithTextCode("FILE_EXTENSION_NOT_FOUND")
	}

	randomName := strconv.FormatInt(time.Now().UnixMicro(), 10)
	imageName := randomName + ext
	if len(paths) > 0 {
		return paths[0] + "/" + imageName, nil
	}

	return imageName, nil
}
