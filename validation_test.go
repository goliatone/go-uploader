package uploader

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	gerrors "github.com/goliatone/go-errors"
)

func createTestFileHeader(filename, contentType string, size int64, content []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)

	part, _ := writer.CreatePart(h)
	part.Write(content)
	writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(32 << 20)

	fileHeader := form.File["file"][0]
	fileHeader.Size = size

	return fileHeader
}

func TestNewValidator(t *testing.T) {
	t.Run("default validator", func(t *testing.T) {
		validator := NewValidator()

		if validator == nil {
			t.Fatal("NewValidator returned nil")
		}

		if validator.maxFileSize != DefaultMaxFileSize {
			t.Errorf("Expected max file size %d, got %d", DefaultMaxFileSize, validator.maxFileSize)
		}

		if validator.allowedMimeTypes == nil {
			t.Error("Expected allowed mime types to be set")
		}

		if validator.allowedImageFormats == nil {
			t.Error("Expected allowed image formats to be set")
		}
	})

	t.Run("validator with options", func(t *testing.T) {
		customMaxSize := int64(10 * 1024 * 1024)
		customMimeTypes := map[string]bool{"image/jpeg": true}
		customFormats := map[string]bool{".jpg": true}

		validator := NewValidator(
			WithUploadMaxFileSize(customMaxSize),
			WithAllowedMimeTypes(customMimeTypes),
			WithAllowedImageFormats(customFormats),
		)

		if validator.maxFileSize != customMaxSize {
			t.Errorf("Expected max file size %d, got %d", customMaxSize, validator.maxFileSize)
		}

		if len(validator.allowedMimeTypes) != 1 || !validator.allowedMimeTypes["image/jpeg"] {
			t.Error("Custom mime types not set correctly")
		}

		if len(validator.allowedImageFormats) != 1 || !validator.allowedImageFormats[".jpg"] {
			t.Error("Custom image formats not set correctly")
		}
	})
}

func TestValidatorValidateFile(t *testing.T) {
	validator := NewValidator()

	t.Run("valid file", func(t *testing.T) {
		content := []byte("valid jpeg content")
		fileHeader := createTestFileHeader("test.jpg", "image/jpeg", 1024, content)

		err := validator.ValidateFile(fileHeader)
		if err != nil {
			t.Fatalf("ValidateFile failed for valid file: %v", err)
		}
	})

	t.Run("file too large", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "image/jpeg", DefaultMaxFileSize+1, content)

		err := validator.ValidateFile(fileHeader)
		if err == nil {
			t.Fatal("Expected error for file too large")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}

		validationErrs, ok := gerrors.GetValidationErrors(err)
		if !ok {
			t.Error("Expected validation errors")
		}

		found := false
		for _, fieldErr := range validationErrs {
			if fieldErr.Field == "file_size" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected file_size validation error")
		}
	})

	t.Run("invalid file format", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.txt", "image/jpeg", 1024, content)

		err := validator.ValidateFile(fileHeader)
		if err == nil {
			t.Fatal("Expected error for invalid file format")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}

		validationErrs, ok := gerrors.GetValidationErrors(err)
		if !ok {
			t.Error("Expected validation errors")
		}

		found := false
		for _, fieldErr := range validationErrs {
			if fieldErr.Field == "file_format" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected file_format validation error")
		}
	})

	t.Run("invalid mime type", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "text/plain", 1024, content)

		err := validator.ValidateFile(fileHeader)
		if err == nil {
			t.Fatal("Expected error for invalid mime type")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}

		validationErrs, ok := gerrors.GetValidationErrors(err)
		if !ok {
			t.Error("Expected validation errors")
		}

		found := false
		for _, fieldErr := range validationErrs {
			if fieldErr.Field == "content_type" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected content_type validation error")
		}
	})
}

func TestValidatorValidateFileContent(t *testing.T) {
	validator := NewValidator()

	t.Run("valid content", func(t *testing.T) {
		jpegHeader := []byte{0xFF, 0xD8, 0xFF}
		content := append(jpegHeader, []byte("jpeg content")...)

		err := validator.ValidateFileContent(content)
		if err != nil {
			t.Fatalf("ValidateFileContent failed for valid content: %v", err)
		}
	})

	t.Run("content too large", func(t *testing.T) {
		content := make([]byte, DefaultMaxFileSize+1)

		err := validator.ValidateFileContent(content)
		if err == nil {
			t.Fatal("Expected error for content too large")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		content := []byte("invalid content")

		err := validator.ValidateFileContent(content)
		if err == nil {
			t.Fatal("Expected error for invalid content")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})
}

func TestValidatorRandomName(t *testing.T) {
	validator := NewValidator()

	t.Run("with path", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "image/jpeg", 1024, content)

		name, err := validator.RandomName(fileHeader, "uploads")
		if err != nil {
			t.Fatalf("RandomName failed: %v", err)
		}

		if !strings.HasPrefix(name, "uploads/") {
			t.Errorf("Expected name to start with 'uploads/', got '%s'", name)
		}

		if !strings.HasSuffix(name, ".jpg") {
			t.Errorf("Expected name to end with '.jpg', got '%s'", name)
		}
	})

	t.Run("without path", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.png", "image/png", 1024, content)

		name, err := validator.RandomName(fileHeader)
		if err != nil {
			t.Fatalf("RandomName failed: %v", err)
		}

		if !strings.HasSuffix(name, ".png") {
			t.Errorf("Expected name to end with '.png', got '%s'", name)
		}

		if strings.Contains(name, "/") {
			t.Errorf("Expected no path separator in name, got '%s'", name)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.gif", "image/gif", 1024, content)

		name, err := validator.RandomName(fileHeader, "")
		if err != nil {
			t.Fatalf("RandomName failed: %v", err)
		}

		if !strings.HasSuffix(name, ".gif") {
			t.Errorf("Expected name to end with '.gif', got '%s'", name)
		}

		if strings.Contains(name, "/") {
			t.Errorf("Expected no path separator in name, got '%s'", name)
		}
	})

	t.Run("no extension", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test", "image/jpeg", 1024, content)

		_, err := validator.RandomName(fileHeader)
		if err == nil {
			t.Fatal("Expected error for file without extension")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})
}

func TestGetAllowedMsg(t *testing.T) {
	options := map[string]bool{
		".jpg":  true,
		".png":  true,
		".gif":  false,
		".webp": true,
	}

	result := getAllowedMsg(options)

	if !strings.Contains(result, ".jpg") {
		t.Error("Result should contain '.jpg'")
	}
	if !strings.Contains(result, ".png") {
		t.Error("Result should contain '.png'")
	}
	if !strings.Contains(result, ".webp") {
		t.Error("Result should contain '.webp'")
	}
	if strings.Contains(result, ".gif") {
		t.Error("Result should not contain '.gif'")
	}
}

func TestValidateFileFunction(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "image/jpeg", 1024, content)

		err := ValidateFile(fileHeader)
		if err != nil {
			t.Fatalf("ValidateFile failed for valid file: %v", err)
		}
	})

	t.Run("file too large", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "image/jpeg", DefaultMaxFileSize+1, content)

		err := ValidateFile(fileHeader)
		if err == nil {
			t.Fatal("Expected error for file too large")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.txt", "image/jpeg", 1024, content)

		err := ValidateFile(fileHeader)
		if err == nil {
			t.Fatal("Expected error for invalid format")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})

	t.Run("invalid mime type", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "text/plain", 1024, content)

		err := ValidateFile(fileHeader)
		if err == nil {
			t.Fatal("Expected error for invalid mime type")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})
}

func TestValidateFileContentFunction(t *testing.T) {
	t.Run("valid content", func(t *testing.T) {
		jpegHeader := []byte{0xFF, 0xD8, 0xFF}
		content := append(jpegHeader, []byte("jpeg content")...)

		err := ValidateFileContent(content)
		if err != nil {
			t.Fatalf("ValidateFileContent failed for valid content: %v", err)
		}
	})

	t.Run("content too large", func(t *testing.T) {
		content := make([]byte, DefaultMaxFileSize+1)

		err := ValidateFileContent(content)
		if err == nil {
			t.Fatal("Expected error for content too large")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		content := []byte("invalid content")

		err := ValidateFileContent(content)
		if err == nil {
			t.Fatal("Expected error for invalid content")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})
}

func TestRandomNameFunction(t *testing.T) {
	t.Run("with path", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.jpg", "image/jpeg", 1024, content)

		name, err := RandomName(fileHeader, "uploads")
		if err != nil {
			t.Fatalf("RandomName failed: %v", err)
		}

		if !strings.HasPrefix(name, "uploads/") {
			t.Errorf("Expected name to start with 'uploads/', got '%s'", name)
		}

		if !strings.HasSuffix(name, ".jpg") {
			t.Errorf("Expected name to end with '.jpg', got '%s'", name)
		}
	})

	t.Run("without path", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test.png", "image/png", 1024, content)

		name, err := RandomName(fileHeader)
		if err != nil {
			t.Fatalf("RandomName failed: %v", err)
		}

		if !strings.HasSuffix(name, ".png") {
			t.Errorf("Expected name to end with '.png', got '%s'", name)
		}

		if strings.Contains(name, "/") {
			t.Errorf("Expected no path separator in name, got '%s'", name)
		}
	})

	t.Run("no extension", func(t *testing.T) {
		content := []byte("test content")
		fileHeader := createTestFileHeader("test", "image/jpeg", 1024, content)

		_, err := RandomName(fileHeader)
		if err == nil {
			t.Fatal("Expected error for file without extension")
		}

		if !gerrors.IsValidation(err) {
			t.Errorf("Expected validation error, got %v", err)
		}
	})
}

func TestIsValidFileContent(t *testing.T) {
	t.Run("valid JPEG", func(t *testing.T) {
		jpegHeader := []byte{0xFF, 0xD8, 0xFF}
		content := append(jpegHeader, []byte("jpeg content")...)

		if !isValidFileContent(content) {
			t.Error("Expected valid JPEG content to be valid")
		}
	})

	t.Run("valid PNG", func(t *testing.T) {
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47}
		content := append(pngHeader, []byte("png content")...)

		if !isValidFileContent(content) {
			t.Error("Expected valid PNG content to be valid")
		}
	})

	t.Run("valid GIF", func(t *testing.T) {
		gifHeader := []byte{0x47, 0x49, 0x46, 0x38}
		content := append(gifHeader, []byte("gif content")...)

		if !isValidFileContent(content) {
			t.Error("Expected valid GIF content to be valid")
		}
	})

	t.Run("valid BMP", func(t *testing.T) {
		bmpHeader := []byte{0x42, 0x4D}
		content := append(bmpHeader, []byte("bmp content")...)

		if !isValidFileContent(content) {
			t.Error("Expected valid BMP content to be valid")
		}
	})

	t.Run("valid WEBP", func(t *testing.T) {
		webpHeader := []byte{0x52, 0x49, 0x46, 0x46}
		content := append(webpHeader, []byte("webp content")...)

		if !isValidFileContent(content) {
			t.Error("Expected valid WEBP content to be valid")
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		content := []byte("invalid content")

		if isValidFileContent(content) {
			t.Error("Expected invalid content to be invalid")
		}
	})

	t.Run("too short content", func(t *testing.T) {
		content := []byte{0x12}

		if isValidFileContent(content) {
			t.Error("Expected too short content to be invalid")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		content := []byte{}

		if isValidFileContent(content) {
			t.Error("Expected empty content to be invalid")
		}
	})
}

func TestCompareBytes(t *testing.T) {
	t.Run("equal bytes", func(t *testing.T) {
		a := []byte{1, 2, 3, 4}
		b := []byte{1, 2, 3, 4}

		if !compareBytes(a, b) {
			t.Error("Expected equal bytes to be equal")
		}
	})

	t.Run("different bytes", func(t *testing.T) {
		a := []byte{1, 2, 3, 4}
		b := []byte{1, 2, 3, 5}

		if compareBytes(a, b) {
			t.Error("Expected different bytes to be not equal")
		}
	})

	t.Run("different lengths", func(t *testing.T) {
		a := []byte{1, 2, 3}
		b := []byte{1, 2, 3, 4}

		if compareBytes(a, b) {
			t.Error("Expected different length bytes to be not equal")
		}
	})

	t.Run("empty slices", func(t *testing.T) {
		a := []byte{}
		b := []byte{}

		if !compareBytes(a, b) {
			t.Error("Expected empty slices to be equal")
		}
	})
}

func TestValidatorOptions(t *testing.T) {
	t.Run("WithUploadMaxFileSize", func(t *testing.T) {
		customSize := int64(5 * 1024 * 1024)
		validator := &Validator{}

		WithUploadMaxFileSize(customSize)(validator)

		if validator.maxFileSize != customSize {
			t.Errorf("Expected max file size %d, got %d", customSize, validator.maxFileSize)
		}
	})

	t.Run("WithAllowedMimeTypes", func(t *testing.T) {
		customTypes := map[string]bool{"image/jpeg": true, "image/png": true}
		validator := &Validator{}

		WithAllowedMimeTypes(customTypes)(validator)

		if validator.allowedMimeTypes == nil {
			t.Error("Expected custom mime types to be set")
		}
	})

	t.Run("WithAllowedImageFormats", func(t *testing.T) {
		customFormats := map[string]bool{".jpg": true, ".png": true}
		validator := &Validator{}

		WithAllowedImageFormats(customFormats)(validator)

		if validator.allowedImageFormats == nil {
			t.Error("Expected custom image formats to be set")
		}
	})
}
