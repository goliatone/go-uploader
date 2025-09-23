# go-uploader

File upload library for Go with multiple storage backends.

## Overview

`go-uploader` provides a unified interface for file uploads with support for AWS S3, local filesystem, and hybrid storage patterns. The library implements a provider pattern where different storage backends conform to the same `Uploader` interface.

## Installation

```bash
go get github.com/goliatone/go-uploader
```

## Quick Start

### Filesystem Provider

```go
package main

import (
    "context"
    "fmt"

    "github.com/goliatone/go-uploader"
)

func main() {
    // Create filesystem provider
    provider := uploader.NewFSProvider("/uploads")

    // Create manager with validation
    manager := uploader.NewManager(provider).
        WithMaxFileSize(10 * 1024 * 1024). // 10MB
        WithAllowedTypes([]string{"image/jpeg", "image/png"})

    // Upload file
    url, err := manager.UploadFile(context.Background(), "avatar.jpg", fileData)
    if err != nil {
        panic(err)
    }

    fmt.Printf("File uploaded: %s\n", url)
}
```

### AWS S3 Provider

```go
package main

import (
    "context"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/goliatone/go-uploader"
)

func main() {
    // Load AWS config
    cfg, err := config.LoadDefaultConfig(context.Background())
    if err != nil {
        panic(err)
    }

    // Create S3 client and provider
    client := s3.NewFromConfig(cfg)
    provider := uploader.NewAWSProvider(client, "my-bucket").
        WithBasePath("uploads/")

    // Create manager
    manager := uploader.NewManager(provider)

    // Upload with metadata
    url, err := manager.UploadFile(
        context.Background(),
        "document.pdf",
        fileData,
        uploader.WithContentType("application/pdf"),
        uploader.WithPublicAccess(true),
    )
    if err != nil {
        panic(err)
    }
}
```

## Core Interface

```go
type Uploader interface {
    UploadFile(ctx context.Context, path string, content []byte, opts ...UploadOption) (string, error)
    GetFile(ctx context.Context, path string) ([]byte, error)
    DeleteFile(ctx context.Context, path string) error
    GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error)
}
```

## Providers

### FSProvider
- Stores files on local filesystem
- Uses Go's `fs.FS` interface for abstraction
- URL generation for web serving

### AWSProvider
- Stores files in AWS S3
- Supports presigned URLs
- Configurable ACLs and metadata

### MultiProvider
- Hybrid storage: local caching + remote storage
- Automatic fallback and synchronization
- Configurable storage strategies

## Validation

```go
manager := uploader.NewManager(provider).
    WithMaxFileSize(5 * 1024 * 1024).                    // 5MB limit
    WithAllowedTypes([]string{"image/jpeg", "image/png"}). // MIME types
    WithAllowedExtensions([]string{".jpg", ".png"}).     // File extensions
    WithValidator(customValidator)                        // Custom validation
```

## Upload Options

```go
url, err := manager.UploadFile(ctx, "file.jpg", data,
    uploader.WithContentType("image/jpeg"),
    uploader.WithCacheControl("max-age=3600"),
    uploader.WithPublicAccess(true),
    uploader.WithTTL(24 * time.Hour),
)
```

## Error Handling

The library uses structured error handling with categorized errors:

```go
if gerrors.IsValidation(err) {
    // Handle validation errors
    if validationErrs, ok := gerrors.GetValidationErrors(err); ok {
        for _, fieldErr := range validationErrs {
            fmt.Printf("Field %s: %s\n", fieldErr.Field, fieldErr.Message)
        }
    }
}

if gerrors.IsNotFound(err) {
    // Handle file not found
}
```

## Examples

See the `examples/` directory for complete implementations:

- `examples/router/`: HTTP server with file upload endpoints using Fiber

## Dependencies

- `github.com/aws/aws-sdk-go-v2`: AWS S3 integration
- `github.com/goliatone/go-errors`: Structured error handling
- `github.com/jszwec/s3fs/v2`: S3 filesystem abstraction

## License

MIT
