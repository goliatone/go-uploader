package uploader

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"
)

// LocalImageProcessor resizes images using a simple nearest-neighbor algorithm.
type LocalImageProcessor struct{}

func NewLocalImageProcessor() *LocalImageProcessor {
	return &LocalImageProcessor{}
}

func (p *LocalImageProcessor) Generate(ctx context.Context, source []byte, size ThumbnailSize, contentType string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	if len(source) == 0 {
		return nil, "", fmt.Errorf("image processor: source is empty")
	}

	img, format, err := decodeImage(bytes.NewReader(source))
	if err != nil {
		return nil, "", err
	}

	target := resizeImage(img, size)

	buf := &bytes.Buffer{}
	mime := contentType
	if mime == "" {
		mime = "image/" + format
	}

	switch format {
	case "jpeg", "jpg":
		if err := jpeg.Encode(buf, target, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", err
		}
		if mime == "" {
			mime = "image/jpeg"
		}
	case "png":
		if err := png.Encode(buf, target); err != nil {
			return nil, "", err
		}
		if mime == "" {
			mime = "image/png"
		}
	case "gif":
		if err := gif.Encode(buf, target, nil); err != nil {
			return nil, "", err
		}
		if mime == "" {
			mime = "image/gif"
		}
	default:
		if err := png.Encode(buf, target); err != nil {
			return nil, "", err
		}
		mime = "image/png"
	}

	return buf.Bytes(), mime, nil
}

func resizeImage(src image.Image, size ThumbnailSize) *image.NRGBA {
	fit := strings.ToLower(size.Fit)
	switch fit {
	case "cover", "outside":
		return resizeCover(src, size.Width, size.Height)
	case "fill":
		return resizeFill(src, size.Width, size.Height)
	case "contain", "inside":
		fallthrough
	default:
		return resizeContain(src, size.Width, size.Height)
	}
}

func resizeFill(src image.Image, width, height int) *image.NRGBA {
	return resizeNearest(src, width, height)
}

func resizeContain(src image.Image, width, height int) *image.NRGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scale := math.Min(float64(width)/float64(srcW), float64(height)/float64(srcH))
	if scale > 1 {
		scale = 1
	}

	newW := int(math.Round(float64(srcW) * scale))
	newH := int(math.Round(float64(srcH) * scale))
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	scaled := resizeNearest(src, newW, newH)
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.NRGBA{A: 0}}, image.Point{}, draw.Src)

	offset := image.Pt((width-newW)/2, (height-newH)/2)
	draw.Draw(canvas, scaled.Bounds().Add(offset), scaled, scaled.Bounds().Min, draw.Src)
	return canvas
}

func resizeCover(src image.Image, width, height int) *image.NRGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scale := math.Max(float64(width)/float64(srcW), float64(height)/float64(srcH))
	newW := int(math.Ceil(float64(srcW) * scale))
	newH := int(math.Ceil(float64(srcH) * scale))

	scaled := resizeNearest(src, newW, newH)
	return cropCenter(scaled, width, height)
}

func cropCenter(img *image.NRGBA, width, height int) *image.NRGBA {
	if img.Bounds().Dx() == width && img.Bounds().Dy() == height {
		return img
	}

	startX := (img.Bounds().Dx() - width) / 2
	startY := (img.Bounds().Dy() - height) / 2
	rect := image.Rect(0, 0, width, height)
	out := image.NewNRGBA(rect)
	draw.Draw(out, rect, img, image.Pt(startX, startY), draw.Src)
	return out
}

func resizeNearest(src image.Image, width, height int) *image.NRGBA {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	srcBounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		sy := srcBounds.Min.Y + int(float64(y)*float64(srcBounds.Dy())/float64(height))
		if sy >= srcBounds.Max.Y {
			sy = srcBounds.Max.Y - 1
		}
		for x := 0; x < width; x++ {
			sx := srcBounds.Min.X + int(float64(x)*float64(srcBounds.Dx())/float64(width))
			if sx >= srcBounds.Max.X {
				sx = srcBounds.Max.X - 1
			}
			dst.Set(x, y, src.At(sx, sy))
		}
	}

	return dst
}

func decodeImage(r io.Reader) (image.Image, string, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("image processor: decode image: %w", err)
	}
	return img, strings.ToLower(format), nil
}
