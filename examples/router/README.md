# File Upload Service Example

This example demonstrates how to use the `go-uploader` library with the `go-router` package to create a complete file upload service.

## Features

- **File Upload API**: Upload files with validation
- **File Management**: Download, delete, and get presigned URLs for files
- **Type Validation**: Supports images (JPEG, PNG, GIF, WebP), PDFs, and text files
- **Size Limits**: 10MB maximum file size
- **OpenAPI Documentation**: Auto-generated API docs
- **Static File Serving**: Direct file access via HTTP

## Quick Start

1. **Install dependencies:**
```bash
go mod tidy
```

2. **Run the server:**
```bash
go run main.go
```

3. **Access the service:**
   - API: http://localhost:9092/api/uploads
   - Documentation: http://localhost:9092/docs
   - Static Files: http://localhost:9092/files

## API Endpoints

### Upload File
```bash
POST /api/uploads
```

Upload a file with optional path specification:

```bash
curl -X POST http://localhost:9092/api/uploads \
  -F "file=@example.jpg" \
  -F "file_path=images"
```

**Response:**
```json
{
  "data": {
    "content_type": "image/jpeg",
    "name": "example.jpg",
    "original_name": "example.jpg",
    "size": 1024000,
    "url": "/files/example.jpg"
  },
  "success": true,
  "message": "File uploaded successfully"
}
```

### Download File
```bash
GET /api/uploads/:filename
```

Download an uploaded file:

```bash
curl -O http://localhost:9092/api/uploads/example.jpg
```

### Delete File
```bash
DELETE /api/uploads/:filename
```

Delete an uploaded file:

```bash
curl -X DELETE http://localhost:9092/api/uploads/example.jpg
```

### Get Presigned URL
```bash
GET /api/uploads/:filename/url
```

Get a presigned URL for file access:

```bash
curl http://localhost:9092/api/uploads/example.jpg/url
```

**Response:**
```json
{
  "data": {
    "url": "/files/example.jpg",
    "filename": "example.jpg",
    "expires": "2025-01-21T15:30:00Z"
  },
  "success": true
}
```

## Configuration

The example uses a filesystem provider by default, storing files in the `./uploads` directory. You can easily switch to other providers:

### AWS S3 Provider
```go
s3Provider := uploader.NewAWSProvider(
    uploader.WithS3Client(s3Client),
    uploader.WithBucket("your-bucket-name"),
)

manager := uploader.NewManager(
    uploader.WithProvider(s3Provider),
    // ... other options
)
```

### Multi Provider (Hybrid)
```go
multiProvider := uploader.NewMultiProvider(
    uploader.WithLocalProvider(fsProvider),
    uploader.WithRemoteProvider(s3Provider),
)

manager := uploader.NewManager(
    uploader.WithProvider(multiProvider),
    // ... other options
)
```

## Validation Options

Customize file validation rules:

```go
validator := uploader.NewValidator(
    uploader.WithMaxFileSize(50*1024*1024), // 50MB
    uploader.WithAllowedTypes([]string{
        "image/jpeg", "image/png",
        "application/pdf",
        "video/mp4",
    }),
    uploader.WithMinFileSize(1024), // 1KB minimum
)

manager := uploader.NewManager(
    uploader.WithValidator(validator),
    // ... other options
)
```

## Error Handling

The service uses structured error handling with appropriate HTTP status codes:

- **400 Bad Request**: Invalid request or missing file
- **413 Payload Too Large**: File exceeds size limit
- **415 Unsupported Media Type**: File type not allowed
- **404 Not Found**: File not found
- **422 Unprocessable Entity**: Validation errors

All errors include detailed information and are properly formatted for API consumption.

## Testing

Test the upload functionality:

```bash
# Upload an image
curl -X POST http://localhost:9092/api/uploads \
  -F "file=@test.jpg"

# Verify upload
curl http://localhost:9092/api/uploads/test.jpg

# Clean up
curl -X DELETE http://localhost:9092/api/uploads/test.jpg
```