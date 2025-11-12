package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goliatone/go-uploader"
)

func main() {
	ctx := context.Background()
	provider := &demoPresignProvider{}
	manager := uploader.NewManager(uploader.WithProvider(provider))

	post, err := manager.CreatePresignedPost(ctx, "uploads/demo.txt",
		uploader.WithContentType("text/plain"),
		uploader.WithTTL(5*time.Minute),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Upload form action:", post.URL)
	for field, value := range post.Fields {
		fmt.Printf("Field %s=%s\n", field, value)
	}

	meta, err := manager.ConfirmPresignedUpload(ctx, &uploader.PresignedUploadResult{
		Key:         "uploads/demo.txt",
		Size:        128,
		ContentType: "text/plain",
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Upload confirmed for %s (%d bytes)\n", meta.Name, meta.Size)
}

type demoPresignProvider struct{}

func (d *demoPresignProvider) UploadFile(context.Context, string, []byte, ...uploader.UploadOption) (string, error) {
	return "", nil
}

func (d *demoPresignProvider) GetFile(context.Context, string) ([]byte, error) { return nil, nil }

func (d *demoPresignProvider) DeleteFile(context.Context, string) error { return nil }

func (d *demoPresignProvider) GetPresignedURL(context.Context, string, time.Duration) (string, error) {
	return "https://files.example.com/tmp/demo.txt", nil
}

func (d *demoPresignProvider) CreatePresignedPost(context.Context, string, *uploader.Metadata) (*uploader.PresignedPost, error) {
	return &uploader.PresignedPost{
		URL:    "https://upload.example.com/form",
		Method: "POST",
		Fields: map[string]string{
			"key":   "uploads/demo.txt",
			"token": "demo-token",
		},
		Expiry: time.Now().Add(5 * time.Minute),
	}, nil
}
