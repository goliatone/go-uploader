package main

import (
	"bytes"
	"context"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/goliatone/go-uploader"
)

func main() {
	ctx := context.Background()
	dir := filepath.Join(os.TempDir(), "go-uploader-callbacks")
	_ = os.MkdirAll(dir, 0o755)

	provider := uploader.NewFSProvider(dir)
	manager := uploader.NewManager(
		uploader.WithProvider(provider),
		uploader.WithOnUploadComplete(func(ctx context.Context, meta *uploader.FileMeta) error {
			log.Printf("callback -> stored %s (%d bytes)", meta.Name, meta.Size)
			return nil
		}),
	)

	header := buildTempFile()
	if _, err := manager.HandleFile(ctx, header, "callbacks"); err != nil {
		panic(err)
	}
}

func buildTempFile() *multipart.FileHeader {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, _ := writer.CreateFormFile("file", "callback.txt")
	part.Write([]byte("callback demo"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	_ = req.ParseMultipartForm(int64(buf.Len()))
	header := req.MultipartForm.File["file"][0]
	header.Header.Set("Content-Type", "text/plain")
	return header
}
