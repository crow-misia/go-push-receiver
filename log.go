package pushreceiver

import (
	"context"
	"log/slog"
)

type noOpHandler struct{}

func (h noOpHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

func (h noOpHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (h noOpHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h noOpHandler) WithGroup(_ string) slog.Handler {
	return h
}
