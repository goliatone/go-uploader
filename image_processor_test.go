package uploader

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestLocalImageProcessorGenerate(t *testing.T) {
	processor := NewLocalImageProcessor()
	src := createTestPNG(40, 20)
	size := ThumbnailSize{Name: "thumb", Width: 10, Height: 10, Fit: "cover"}

	thumb, mime, err := processor.Generate(context.Background(), src, size, "image/png")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if mime != "image/png" {
		t.Fatalf("expected image/png, got %s", mime)
	}

	img, _, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}

	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Fatalf("expected 10x10 thumbnail, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func createTestPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 5), G: uint8(y * 5), B: 0x80, A: 0xff})
		}
	}

	buf := &bytes.Buffer{}
	_ = png.Encode(buf, img)
	return buf.Bytes()
}
