package uploader

import "log"

// Logger interface allows for dependency injection of logging
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

type DefaultLogger struct{}

func (l *DefaultLogger) Info(msg string, args ...any) {
	log.Printf(" [INFO] Search | "+msg+"\n", args...)
}

func (l *DefaultLogger) Error(msg string, args ...any) {
	log.Printf("[ERROR] Search | "+msg+"\n", args...)
}
