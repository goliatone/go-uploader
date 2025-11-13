# go-uploader

File upload library for Go with multiple storage backends.

## Overview

`go-uploader` provides a unified interface for file uploads with support for AWS S3, local filesystem, and hybrid storage patterns. The library implements a provider pattern where different storage backends conform to the same `Uploader` interface.

## Feature Highlights
- **Chunked uploads** with resumable sessions (`InitiateChunked`, `UploadChunk`, `CompleteChunked`, `AbortChunked`) backed by AWS multipart uploads or filesystem chunk staging. See `examples/chunked`.
- **Direct-to-storage presigned posts** including confirmation helpers so your API only receives metadata. See `examples/presignedpost`.
- **Server-side thumbnails** driven by a pluggable `ImageProcessor`, producing derivative metadata alongside the original asset. See `examples/thumbnails`.
- **Post-upload callbacks** with strict/best-effort modes and async executors for downstream automation. See `examples/callbacks`.

Need deeper design references?

- `FEATURE_TDD.md` – behaviour-driven specs with current status references.
- `docs/FEATURE_DESIGN_NOTES.md` – provider constraints and validation rules.
- `FEATURE_TSK.md` – phased implementation tracker (updated after every phase).

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

## Chunked Uploads

Large files or unreliable networks can use the chunked API, which streams parts to any provider implementing `ChunkedUploader` (AWS S3, filesystem, multi-provider).

```go
session, err := manager.InitiateChunked(ctx, "videos/raw.mov", totalSize)
if err != nil {
    panic(err)
}

for idx, chunk := range parts {
    if err := manager.UploadChunk(ctx, session.ID, idx, bytes.NewReader(chunk)); err != nil {
        panic(err)
    }
}

meta, err := manager.CompleteChunked(ctx, session.ID)
if err != nil {
    panic(err)
}

fmt.Println("Uploaded via chunked API:", meta.URL)
```

Each session tracks expected part counts and expiries inside the manager's registry; both AWS and filesystem providers persist their own IDs so restarts continue safely. Try `go run ./examples/chunked` for a CLI that simulates a UI progress bar and inspects the staged data under `.example-chunks/`.

## Direct to Storage Presigned Posts

Generate presigned POST data so browsers can upload directly to storage, then confirm the asset without proxying the bytes through your API.

```go
post, err := manager.CreatePresignedPost(ctx, "uploads/raw.mov",
    uploader.WithContentType("video/quicktime"),
    uploader.WithTTL(10*time.Minute),
)
if err != nil {
    panic(err)
}

// Send post.URL + post.Fields to the browser for the actual upload.

meta, err := manager.ConfirmPresignedUpload(ctx, &uploader.PresignedUploadResult{
    Key:         "uploads/raw.mov",
    Size:        104857600,
    ContentType: "video/quicktime",
})
if err != nil {
    panic(err)
}
```

Use the generated payload to build a browser form:

```html
<form action="{{ .URL }}" method="post" enctype="multipart/form-data">
  {{ range $k, $v := .Fields }}
    <input type="hidden" name="{{ $k }}" value="{{ $v }}">
  {{ end }}
  <input type="file" name="file">
  <button>Upload</button>
</form>
```

See `examples/presignedpost/` for a runnable CLI plus a ready-to-copy HTML template.

## Server Side Thumbnails

Generate consistent derivatives on the server after validating uploads.

```go
sizes := []uploader.ThumbnailSize{
    {Name: "small", Width: 200, Height: 200, Fit: "cover"},
    {Name: "preview", Width: 800, Height: 600, Fit: "contain"},
}

imageMeta, err := manager.HandleImageWithThumbnails(ctx, fileHeader, "gallery", sizes)
if err != nil {
    panic(err)
}

fmt.Println("Original:", imageMeta.URL)
fmt.Println("Small thumb:", imageMeta.Thumbnails["small"].URL)
```

The default processor is pure Go and can be replaced via `WithImageProcessor` for advanced pipelines.

## Post Upload Callbacks

Register a callback to perform follow-up work (virus scanning, notifications, etc.) after uploads complete.

```go
manager := uploader.NewManager(
    uploader.WithProvider(provider),
    uploader.WithOnUploadComplete(func(ctx context.Context, meta *uploader.FileMeta) error {
        log.Printf("stored %s (%d bytes)", meta.Name, meta.Size)
        return nil
    }),
    uploader.WithCallbackMode(uploader.CallbackModeStrict),
)
```

Callbacks default to best-effort. Use `CallbackModeStrict` to fail uploads when the callback returns an error, or provide `WithCallbackExecutor(NewAsyncCallbackExecutor(nil))` to dispatch work asynchronously.

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

See `examples/README.md` for full walkthroughs. Highlights:
- `examples/router/`: HTTP server (Fiber + go-router) with upload API, gallery view, and presigned URL helper.
- `examples/chunked/`: Chunk UI simulator that spins up chunk sessions, streams parts, and inspects the final artifact.
- `examples/presignedpost/`: Presigned upload form generator plus CLI confirmation flow.
- `examples/thumbnails/`: Thumbnail handler that renders gradients, resizes them, and prints resulting URLs.
- `examples/callbacks/`: Demonstrates callback registration, logging, and strict/best-effort considerations.

## Dependencies

- `github.com/aws/aws-sdk-go-v2`: AWS S3 integration
- `github.com/goliatone/go-errors`: Structured error handling
- `github.com/jszwec/s3fs/v2`: S3 filesystem abstraction

## License
Goliatone MIT
