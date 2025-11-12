package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/goliatone/go-uploader"
)

func main() {
	ctx := context.Background()

	baseDir := "./.example-chunks"
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		panic(err)
	}

	provider := uploader.NewFSProvider(baseDir)
	manager := uploader.NewManager(uploader.WithProvider(provider))

	data := bytes.Repeat([]byte("chunked-upload-"), 32)
	session, err := manager.InitiateChunked(ctx, "demo/chunked.bin", int64(len(data)))
	if err != nil {
		panic(err)
	}

	const partSize = 1024
	chunkCount := 0
	for offset := 0; offset < len(data); offset += partSize {
		end := offset + partSize
		if end > len(data) {
			end = len(data)
		}

		if err := manager.UploadChunk(ctx, session.ID, chunkCount, bytes.NewReader(data[offset:end])); err != nil {
			panic(err)
		}
		chunkCount++
	}

	meta, err := manager.CompleteChunked(ctx, session.ID)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Chunked upload complete: %s (%d bytes)\n", meta.URL, meta.Size)
}
