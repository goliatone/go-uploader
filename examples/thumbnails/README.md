# Thumbnail Handler Demo

`go run ./examples/thumbnails` renders an in-memory gradient, uploads it via the filesystem provider, and generates two derivative sizes (`small`, `preview`) using `HandleImageWithThumbnails`.

## Running the sample
```bash
go run ./examples/thumbnails
```

You should see console output similar to:
```
Original stored at: file:///var/folders/.../gallery/original.png
small thumbnail stored at: file:///.../gallery/original__small.png
preview thumbnail stored at: file:///.../gallery/original__preview.png
```

Check the temp directory (printed in the output) to inspect each file. The example demonstrates:
- `ThumbnailSize` validation (width, height, fit).
- Default image processor usage (pure-Go implementation).
- Returned `ImageMeta` structure for persisting derivative metadata.

Adjust the `sizes` slice or swap in your own processor via `WithImageProcessor` to experiment with different formats before integrating into a production service.
