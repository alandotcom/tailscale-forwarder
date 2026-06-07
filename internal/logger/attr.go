package logger

import (
	"log/slog"
	"strings"
)

// ErrAttr returns a slog attribute carrying the (trimmed) error message under
// the "err" key.
func ErrAttr(err error) slog.Attr {
	return slog.String("err", strings.TrimSpace(err.Error()))
}
