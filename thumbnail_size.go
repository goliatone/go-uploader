package uploader

import (
	"fmt"
	"strings"

	gerrors "github.com/goliatone/go-errors"
)

// allowedThumbnailFits enumerates valid resize behaviors.
var allowedThumbnailFits = map[string]bool{
	"cover":   true,
	"contain": true,
	"fill":    true,
	"inside":  true,
	"outside": true,
}

// ThumbnailSize describes a requested derivative output.
type ThumbnailSize struct {
	Name   string
	Width  int
	Height int
	Fit    string
}

// ValidateThumbnailSizes ensures the configured derivatives are viable.
func ValidateThumbnailSizes(sizes []ThumbnailSize) error {
	if len(sizes) == 0 {
		return gerrors.NewValidation("thumbnail sizes invalid",
			gerrors.FieldError{
				Field:   "sizes",
				Message: "at least one thumbnail size is required",
				Value:   0,
			},
		)
	}

	seen := make(map[string]struct{}, len(sizes))
	for idx, size := range sizes {
		fieldPrefix := fmt.Sprintf("sizes[%d]", idx)
		name := strings.TrimSpace(size.Name)
		if name == "" {
			return gerrors.NewValidation("thumbnail sizes invalid",
				gerrors.FieldError{
					Field:   fieldPrefix + ".name",
					Message: "name cannot be empty",
				},
			)
		}

		lowerName := strings.ToLower(name)
		if _, exists := seen[lowerName]; exists {
			return gerrors.NewValidation("thumbnail sizes invalid",
				gerrors.FieldError{
					Field:   fieldPrefix + ".name",
					Message: "duplicate thumbnail name",
					Value:   name,
				},
			)
		}
		seen[lowerName] = struct{}{}

		if size.Width <= 0 {
			return gerrors.NewValidation("thumbnail sizes invalid",
				gerrors.FieldError{
					Field:   fieldPrefix + ".width",
					Message: "width must be greater than zero",
					Value:   size.Width,
				},
			)
		}

		if size.Height <= 0 {
			return gerrors.NewValidation("thumbnail sizes invalid",
				gerrors.FieldError{
					Field:   fieldPrefix + ".height",
					Message: "height must be greater than zero",
					Value:   size.Height,
				},
			)
		}

		fit := strings.ToLower(strings.TrimSpace(size.Fit))
		if fit == "" {
			return gerrors.NewValidation("thumbnail sizes invalid",
				gerrors.FieldError{
					Field:   fieldPrefix + ".fit",
					Message: "fit must be provided (cover, contain, fill, inside, outside)",
				},
			)
		}

		if !allowedThumbnailFits[fit] {
			return gerrors.NewValidation("thumbnail sizes invalid",
				gerrors.FieldError{
					Field:   fieldPrefix + ".fit",
					Message: "unsupported fit value",
					Value:   size.Fit,
				},
			)
		}
	}

	return nil
}
