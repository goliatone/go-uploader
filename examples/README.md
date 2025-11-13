# go-uploader Examples

Each subdirectory under `examples/` focuses on a recently delivered feature so you can copy/paste a working baseline into your own service. All commands assume you run them from the repository root.

## `router/` – Full HTTP service
Runs a Fiber + go-router HTTP API that exposes upload endpoints, presigned URL helpers, and a gallery view. See `examples/router/README.md` for configuration and how to toggle between filesystem, AWS, and multi-provider deployments.

## `chunked/` – Chunk UI simulator
`go run ./examples/chunked` spins up an in-memory file, initiates a chunk session, and logs each uploaded part so you can observe resumable progress. Pair it with the HTML snippet in `examples/chunked/README.md` to build a drag-and-drop UI that talks to your own API.

## `presignedpost/` – Direct upload form
Generates a presigned POST payload plus a ready-to-copy `<form>` template (`form.html`). After pushing the file through your browser, call `ConfirmPresignedUpload` (demonstrated in `main.go`) so metadata ends up in your database.

## `thumbnails/` – Thumbnail handler
Creates a gradient image at runtime, processes two thumbnail sizes, and prints out the resulting URLs. Useful to understand how `HandleImageWithThumbnails` uses the default processor and how to persist the returned `ImageMeta`.

## `callbacks/` – Callback logging
Registers an upload-complete callback that logs every stored asset. Toggle callback modes or wrap it with `NewAsyncCallbackExecutor` to see strict vs best-effort behaviour during `go run ./examples/callbacks`.

Feel free to mix and match pieces—each example is intentionally small and dependency-light so it can double as runnable documentation.
