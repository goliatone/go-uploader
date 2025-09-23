package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	gconf "github.com/goliatone/go-config/config"
	"github.com/goliatone/go-logger/glog"
	"github.com/goliatone/go-uploader/examples/router/config"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-router"
	"github.com/goliatone/go-uploader"
)

type App struct {
	uploadsManager *uploader.Manager
	logger         uploader.Logger
	cfg            *config.Config
	assetsFS       fs.FS
}

func (a App) Config() *config.Config {
	return a.cfg
}

func (a App) IsDevelopment() bool {
	return a.cfg.GetEnvironment() == "development"
}

func (a *App) SetUploadsManager(umng *uploader.Manager) *App {
	a.uploadsManager = umng
	return a
}

func (a *App) SetAssetsFS(imageFS fs.FS) *App {
	a.assetsFS = imageFS
	return a
}

func (a App) AssetsFS() fs.FS {
	return a.assetsFS
}

func NewApp() *App {

	log := glog.NewLogger(
		glog.WithName("app"),
		glog.WithLevel(glog.Debug),
		glog.WithLoggerTypePretty(),
	)

	cfg := &config.Config{}
	container := gconf.New(cfg).
		WithProvider(gconf.EnvProvider[*config.Config]("APP_", "__")).
		WithProvider(gconf.FileProvider[*config.Config]("./config/app.json")).
		WithLogger(log)

	if err := container.LoadWithDefaults(); err != nil {
		panic(err)
	}

	return &App{
		cfg:    cfg,
		logger: log,
	}
}

func WithUploadService(ctx context.Context, app *App) error {
	cfg := app.Config().Images

	s3Cfg, err := aconfig.LoadDefaultConfig(ctx,
		aconfig.WithRegion(cfg.S3.Region),
		aconfig.WithSharedConfigProfile(cfg.S3.Profile),
	)
	if err != nil {
		return err
	}

	var opts = func(o *s3.Options) {}
	if app.IsDevelopment() {
		opts = func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.S3.EndpointURL)
			o.UsePathStyle = true
		}
	}

	client := s3.NewFromConfig(s3Cfg, opts)
	awsProvider := uploader.NewAWSProvider(client, cfg.S3.Bucket)
	awsProvider.WithLogger(app.Logger("svc.img.aws"))
	awsProvider.WithBasePath(cfg.S3.BasePath)

	localProvider := uploader.NewFSProvider(cfg.Fs.BasePath)
	localProvider.WithLogger(app.Logger("svc.img.fs"))

	multi := uploader.NewMultiProvider(localProvider, awsProvider)

	svc := uploader.NewManager(
		uploader.WithLogger(app.Logger("svc.img")),
		uploader.WithProvider(multi),
	)

	imageFS := uploader.NewFileFS(client, cfg.S3.Bucket)

	// app.SetS3Client(client)
	app.SetAssetsFS(imageFS)
	app.SetUploadsManager(svc)

	return nil
}

func (a *App) UploadsManager() *uploader.Manager {
	return a.uploadsManager
}

func (a *App) Logger(name string) uploader.Logger {
	return a.logger
}

func createRoutes(server router.Server[*fiber.App], app *App) {
	// Error middleware
	errMiddleware := router.WithErrorHandlerMiddleware(
		router.WithEnvironment("development"),
		router.WithStackTrace(true),
	)

	server.Router().Use(errMiddleware)

	// Root route with upload form and gallery
	server.Router().Get("/", homeHandler(app))

	// API routes
	api := server.Router().Group("/api")
	api.Use(router.ToMiddleware(func(c router.Context) error {
		c.SetHeader(router.HeaderContentType, "application/json")
		return c.Next()
	}))

	builder := router.NewRouteBuilder(api)

	// File upload routes
	uploads := builder.Group("/uploads")
	{
		uploads.NewRoute().
			POST().
			Path("/").
			Summary("Upload File").
			Description(`## Upload File
Upload a file to the server. Supports various file types including images, PDFs, and text files.

**Supported file types:**
- Images: JPEG, PNG, GIF, WebP
- Documents: PDF, Text files

**Maximum file size:** 10MB
			`).
			Tags("Upload").
			Handler(uploadFileHandler(app)).
			Name("upload.file")

		uploads.NewRoute().
			GET().
			Path("/:filename").
			Summary("Get File").
			Description("Retrieve an uploaded file by filename").
			Tags("Upload").
			Handler(getFileHandler(app)).
			Name("upload.get")

		uploads.NewRoute().
			DELETE().
			Path("/:filename").
			Summary("Delete File").
			Description("Delete an uploaded file by filename").
			Tags("Upload").
			Handler(deleteFileHandler(app)).
			Name("upload.delete")

		uploads.NewRoute().
			GET().
			Path("/:filename/url").
			Summary("Get Presigned URL").
			Description("Get a presigned URL for file access (useful for S3 provider)").
			Tags("Upload").
			Handler(getPresignedURLHandler(app)).
			Name("upload.presigned")

		uploads.BuildAll()
	}

	// Health check
	builder.NewRoute().
		GET().
		Path("/health").
		Description("Health endpoint").
		Tags("Health").
		Handler(healthHandler).
		Name("health")

	builder.BuildAll()
}

func uploadFileHandler(app *App) router.HandlerFunc {
	return func(ctx router.Context) error {
		app.Logger("upload").Info("file upload request")

		// Get file path from form (optional)
		filePath := ctx.FormValue("file_path")
		// With config fs.base_path="uploads", empty filePath means files go to "uploads/"
		// If user specifies a path like "images", files go to "uploads/images/"

		// Get uploaded file
		file, err := ctx.FormFile("file")
		if err != nil {
			app.Logger("upload").Error("failed to get file", err)
			return router.NewBadRequestError("No file provided or invalid file")
		}

		// Handle file upload using uploader manager
		meta, err := app.UploadsManager().HandleFile(ctx.Context(), file, filePath)
		if err != nil {
			app.Logger("upload").Error("failed to handle file", err)
			return err // uploader returns structured errors
		}

		return ctx.JSON(http.StatusOK, router.ViewContext{
			"data":    meta,
			"success": true,
			"message": "File uploaded successfully",
		})
	}
}

func getFileHandler(app *App) router.HandlerFunc {
	return func(ctx router.Context) error {
		filename := ctx.Param("filename", "")
		if filename == "" {
			return router.NewBadRequestError("filename is required")
		}

		// Build file path
		filePath := "uploads/" + filename

		// Get file content
		content, err := app.UploadsManager().GetFile(ctx.Context(), filePath)
		if err != nil {
			app.Logger("upload").Error("failed to get file", err)
			return err
		}

		// Set appropriate headers and return file content
		ctx.SetHeader("Content-Type", "application/octet-stream")
		ctx.SetHeader("Content-Disposition", "attachment; filename="+filename)

		return ctx.Send(content)
	}
}

func deleteFileHandler(app *App) router.HandlerFunc {
	return func(ctx router.Context) error {
		app.Logger("upload").Info("delete file request received")

		filename := ctx.Param("filename", "")
		if filename == "" {
			app.Logger("upload").Error("filename parameter missing")
			return router.NewBadRequestError("filename is required")
		}

		app.Logger("upload").Info("deleting file", "filename", filename)

		// Delete file
		err := app.UploadsManager().DeleteFile(ctx.Context(), filename)
		if err != nil {
			app.Logger("upload").Error("failed to delete file", err)
			return err
		}

		app.Logger("upload").Info("file deleted successfully", "filename", filename)

		return ctx.JSON(http.StatusOK, router.ViewContext{
			"success": true,
			"message": "File deleted successfully",
		})
	}
}

func getPresignedURLHandler(app *App) router.HandlerFunc {
	return func(ctx router.Context) error {
		filename := ctx.Param("filename", "")
		if filename == "" {
			return router.NewBadRequestError("filename is required")
		}

		// Build file path
		filePath := "uploads/" + filename

		// Get presigned URL (expires in 1 hour)
		url, err := app.UploadsManager().GetPresignedURL(ctx.Context(), filePath, time.Hour)
		if err != nil {
			app.Logger("upload").Error("failed to get presigned URL", err)
			return err
		}

		return ctx.JSON(http.StatusOK, router.ViewContext{
			"data": map[string]string{
				"url":      url,
				"filename": filename,
				"expires":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			"success": true,
		})
	}
}

func healthHandler(ctx router.Context) error {
	return ctx.JSON(http.StatusOK, router.ViewContext{
		"status":  "healthy",
		"service": "file-upload-service",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func homeHandler(app *App) router.HandlerFunc {
	return func(ctx router.Context) error {
		// Get list of uploaded files
		files, err := getUploadedFiles(app)
		if err != nil {
			app.Logger("home").Error("failed to get uploaded files", err)
			files = []string{} // Continue with empty list on error
		}

		// Generate HTML page
		html := generateHomePage(files)
		ctx.SetHeader("Content-Type", "text/html")
		return ctx.SendString(html)
	}
}

func getUploadedFiles(app *App) ([]string, error) {
	// Read files from S3 filesystem instead of local directory
	files := []string{}

	// Use the S3 filesystem to list files in the uploads directory
	assetsFS := app.AssetsFS()
	entries, err := fs.ReadDir(assetsFS, "uploads")
	if err != nil {
		return files, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

func generateHomePage(files []string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>File Upload Service</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .upload-form { background: #f5f5f5; padding: 20px; border-radius: 8px; margin-bottom: 30px; }
        .file-gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 20px; }
        .file-item { background: white; border: 1px solid #ddd; border-radius: 8px; padding: 10px; text-align: center; }
        .file-item img { max-width: 100%; height: 150px; object-fit: cover; border-radius: 4px; }
        .file-item .filename { margin-top: 10px; font-size: 14px; word-break: break-word; }
        .file-actions { margin-top: 10px; }
        .file-actions a { margin: 0 5px; padding: 5px 10px; text-decoration: none; background: #007bff; color: white; border-radius: 4px; font-size: 12px; }
        .file-actions a.delete { background: #dc3545; }
        .file-actions a:hover { opacity: 0.8; }
        .upload-btn { background: #28a745; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
        .upload-btn:hover { background: #218838; }
        .form-group { margin-bottom: 15px; padding-right:15px }
        .form-group label { display: block; margin-bottom: 5px; font-weight: bold; }
        .form-group input, .form-group select { width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; }
        .upload-result { margin-top: 15px; padding: 10px; border-radius: 4px; display: none; }
        .upload-result.success { background: #d4edda; color: #155724; border: 1px solid #c3e6cb; }
        .upload-result.error { background: #f8d7da; color: #721c24; border: 1px solid #f5c6cb; }
    </style>
</head>
<body>
    <h1>File Upload Service</h1>

    <div class="upload-form">
        <h2>Upload New File</h2>
        <form id="uploadForm" enctype="multipart/form-data">
            <div class="form-group">
                <label for="file">Choose file:</label>
                <input type="file" id="file" name="file" required>
            </div>
            <div class="form-group">
                <label for="file_path">Upload path (optional):</label>
                <input type="text" id="file_path" name="file_path" placeholder="" value="">
            </div>
            <button type="submit" class="upload-btn">Upload File</button>
        </form>
        <div id="uploadResult" class="upload-result"></div>
    </div>

    <div class="gallery-section">
        <h2>Uploaded Files (` + fmt.Sprintf("%d", len(files)) + ` files)</h2>
        ` + generateFileGallery(files) + `
    </div>

    <script>
        document.getElementById('uploadForm').addEventListener('submit', async function(e) {
            e.preventDefault();

            const formData = new FormData();
            const fileInput = document.getElementById('file');
            const pathInput = document.getElementById('file_path');
            const resultDiv = document.getElementById('uploadResult');

            if (!fileInput.files[0]) {
                showResult('Please select a file to upload', 'error');
                return;
            }

            formData.append('file', fileInput.files[0]);
            formData.append('file_path', pathInput.value || '');

            try {
                const response = await fetch('/api/uploads/', {
                    method: 'POST',
                    body: formData
                });

                const result = await response.json();

                if (response.ok) {
                    showResult('File uploaded successfully: ' + result.data.filename, 'success');
                    // Reload the page after 1 second to show the new file
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    showResult('Upload failed: ' + (result.message || 'Unknown error'), 'error');
                }
            } catch (error) {
                showResult('Upload failed: ' + error.message, 'error');
            }
        });

        function showResult(message, type) {
            const resultDiv = document.getElementById('uploadResult');
            resultDiv.textContent = message;
            resultDiv.className = 'upload-result ' + type;
            resultDiv.style.display = 'block';

            // Hide after 5 seconds
            setTimeout(() => {
                resultDiv.style.display = 'none';
            }, 5000);
        }

        async function deleteFile(filename) {
            if (!confirm('Are you sure you want to delete ' + filename + '?')) {
                return;
            }

            try {
                const response = await fetch('/api/uploads/' + encodeURIComponent(filename), {
                    method: 'DELETE',
                    headers: {
                        'Accept': 'application/json',
                        'Content-Type': 'application/json'
                    }
                });

                if (response.ok) {
                    const result = await response.json();
                    alert('File deleted successfully');
                    window.location.reload();
                } else {
                    const result = await response.json();
                    alert('Delete failed: ' + (result.message || 'Unknown error'));
                }
            } catch (error) {
                console.error('Delete error:', error);
                alert('Delete failed: ' + error.message);
            }
        }
    </script>
</body>
</html>`
}

func generateFileGallery(files []string) string {
	if len(files) == 0 {
		return `<p>No files uploaded yet. Use the form above to upload your first file!</p>`
	}

	gallery := `<div class="file-gallery">`

	for _, filename := range files {
		isImage := isImageFile(filename)

		gallery += `<div class="file-item">`

		if isImage {
			gallery += `<img src="/files/uploads/` + filename + `" alt="` + filename + `" onerror="this.style.display='none'">`
		} else {
			gallery += `<div style="height: 150px; display: flex; align-items: center; justify-content: center; background: #f8f9fa; border: 2px dashed #dee2e6; border-radius: 4px;">`
			gallery += `<span style="font-size: 24px;">📄</span>`
			gallery += `</div>`
		}

		gallery += `<div class="filename">` + filename + `</div>`
		gallery += `<div class="file-actions">`
		gallery += `<a href="/api/uploads/` + filename + `" download>Download</a>`
		gallery += `<a href="#" class="delete" onclick="deleteFile('` + filename + `')">Delete</a>`
		gallery += `</div>`
		gallery += `</div>`
	}

	gallery += `</div>`
	return gallery
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filename)
	return strings.HasSuffix(ext, ".jpg") ||
		strings.HasSuffix(ext, ".jpeg") ||
		strings.HasSuffix(ext, ".png") ||
		strings.HasSuffix(ext, ".gif") ||
		strings.HasSuffix(ext, ".webp") ||
		strings.HasSuffix(ext, ".bmp") ||
		strings.HasSuffix(ext, ".svg")
}

func newFiberAdapter() router.Server[*fiber.App] {
	return router.NewFiberAdapter(func(a *fiber.App) *fiber.App {
		return fiber.New(
			fiber.Config{
				AppName:           "Go Uploader Example",
				EnablePrintRoutes: true,
				PassLocalsToViews: true,
			},
		)
	})
}

func main() {
	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll("./uploads", 0755); err != nil {
		log.Fatalf("Failed to create uploads directory: %v", err)
	}

	// Initialize app and router
	app := NewApp()
	server := newFiberAdapter()

	ctx := context.Background()

	if err := WithUploadService(ctx, app); err != nil {
		panic(err)
	}

	// Setup routes
	createRoutes(server, app)

	// Print routes for debugging
	server.Router().PrintRoutes()

	// Serve static files from S3 filesystem
	server.Router().Static("/files", "/", router.Static{
		FS: app.AssetsFS(),
	})

	// Setup OpenAPI documentation
	front := server.Router().Use(router.ToMiddleware(func(c router.Context) error {
		c.SetHeader(router.HeaderContentType, "text/html")
		return c.Next()
	}))

	router.ServeOpenAPI(front, router.NewOpenAPIRenderer(router.OpenAPIRenderer{
		Title:   "File Upload Service",
		Version: "v1.0.0",
		Description: `## File Upload API
This API provides endpoints for uploading, retrieving, and managing files.

### Features
- **File Upload**: Upload files with validation (size, type)
- **File Retrieval**: Download uploaded files
- **File Management**: Delete files and get presigned URLs
- **Supported Formats**: Images (JPEG, PNG, GIF, WebP), PDFs, Text files
- **File Size Limit**: 10MB maximum

### Usage Examples

**Upload a file:**
` + "```bash" + `
curl -X POST http://localhost:9092/api/uploads \
  -F "file=@example.jpg" \
  -F "file_path=images"
` + "```" + `

**Download a file:**
` + "```bash" + `
curl -O http://localhost:9092/api/uploads/example.jpg
` + "```" + `

**Delete a file:**
` + "```bash" + `
curl -X DELETE http://localhost:9092/api/uploads/example.jpg
` + "```" + `
		`,
		Contact: &router.OpenAPIFieldContact{
			Email: "support@example.com",
			Name:  "File Upload Service",
			URL:   "https://example.com",
		},
	}))

	// Start server
	go func() {
		if err := server.Serve(":9092"); err != nil {
			log.Panic(err)
		}
	}()

	app.Logger("app").Info("🚀 File Upload Service started on http://localhost:9092")
	app.Logger("app").Info("📖 API Documentation available at http://localhost:9092/docs")
	app.Logger("app").Info("📁 Static files served at http://localhost:9092/files")

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	app.Logger("app").Info("🛑 Shutting down server...")

	if err := server.Shutdown(ctx); err != nil {
		log.Panic(err)
	}
}
