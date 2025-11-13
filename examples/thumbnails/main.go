package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/goliatone/go-uploader"
)

func main() {
	ctx := context.Background()
	dir := filepath.Join(os.TempDir(), "go-uploader-thumbnails")
	_ = os.MkdirAll(dir, 0o755)

	provider := uploader.NewFSProvider(dir)
	manager := uploader.NewManager(uploader.WithProvider(provider))

	fileHeader := buildSampleFileHeader("preview.png")

	sizes := []uploader.ThumbnailSize{
		{Name: "small", Width: 64, Height: 64, Fit: "cover"},
		{Name: "preview", Width: 320, Height: 200, Fit: "contain"},
	}

	meta, err := manager.HandleImageWithThumbnails(ctx, fileHeader, "gallery", sizes)
	if err != nil {
		panic(err)
	}

	fmt.Println("Original stored at:", meta.URL)
	for name, thumb := range meta.Thumbnails {
		fmt.Printf("%s thumbnail stored at: %s\n", name, thumb.URL)
	}
}

func buildSampleFileHeader(filename string) *multipart.FileHeader {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, _ := writer.CreateFormFile("file", filename)

	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	for y := 0; y < 150; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	_ = png.Encode(part, img)
	writer.Close()

	reader := multipart.NewReader(bytes.NewReader(buf.Bytes()), writer.Boundary())
	form, _ := reader.ReadForm(int64(buf.Len()))
	fh := form.File["file"][0]
	fh.Header.Set("Content-Type", "image/png")
	fh.Size = int64(buf.Len())
	return fh
}
