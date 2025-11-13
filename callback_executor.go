package uploader

import "context"

type CallbackExecutor interface {
	Execute(ctx context.Context, cb UploadCallback, meta *FileMeta) error
}

type syncCallbackExecutor struct{}

func (syncCallbackExecutor) Execute(ctx context.Context, cb UploadCallback, meta *FileMeta) error {
	return cb(ctx, meta)
}

type AsyncCallbackExecutor struct {
	logger Logger
}

func NewAsyncCallbackExecutor(logger Logger) *AsyncCallbackExecutor {
	if logger == nil {
		logger = &DefaultLogger{}
	}
	return &AsyncCallbackExecutor{logger: logger}
}

func (e *AsyncCallbackExecutor) Execute(ctx context.Context, cb UploadCallback, meta *FileMeta) error {
	if cb == nil || meta == nil {
		return nil
	}

	go func() {
		if err := cb(ctx, meta); err != nil && e.logger != nil {
			e.logger.Error("async upload callback failed", err, "key", meta.Name)
		}
	}()

	return nil
}
