# Post-upload Callback Demo

Use this example to validate callback configuration, logging behaviour, and strict/best-effort modes.

## Run the sample
```bash
go run ./examples/callbacks
```

The program:
1. Creates a temporary filesystem provider.
2. Registers `WithOnUploadComplete` to log every stored file.
3. Streams a fake multipart upload through `HandleFile`.

Expected log output:
```
callback -> stored callbacks/callback.txt (13 bytes)
```

## Experiment with callback modes
Open `main.go` and toggle:
- `uploader.WithCallbackMode(uploader.CallbackModeStrict)` to fail uploads when the callback errors.
- `uploader.WithCallbackExecutor(uploader.NewAsyncCallbackExecutor(nil))` to run callbacks asynchronously.

Combine this example with `uploader_callback_test.go` when extending the callback API—having both a runnable binary and automated tests keeps regressions from sneaking in.
