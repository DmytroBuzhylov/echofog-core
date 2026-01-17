package logger

import (
	"context"
	"log/slog"
)

type LogEntry struct {
	Time    string            `json:"time"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}

type ChannelHandler struct {
	out         chan<- LogEntry
	opts        slog.HandlerOptions
	attrs       []slog.Attr
	groupPrefix string
}

func NewChannelHandler(out chan<- LogEntry, opts *slog.HandlerOptions) *ChannelHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &ChannelHandler{
		out:  out,
		opts: *opts,
	}
}

func (h *ChannelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *ChannelHandler) Handle(ctx context.Context, r slog.Record) error {
	entry := LogEntry{
		Time:    r.Time.Format("15:04:05"),
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   make(map[string]string),
	}

	r.Attrs(func(a slog.Attr) bool {
		entry.Attrs[a.Key] = a.Value.String()
		return true
	})

	for _, a := range h.attrs {
		entry.Attrs[a.Key] = a.Value.String()
	}

	select {
	case h.out <- entry:
	default:
	}

	return nil
}

func (h *ChannelHandler) WithGroup(name string) slog.Handler {
	return h
}

func (h *ChannelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ChannelHandler{
		out:   h.out,
		opts:  h.opts,
		attrs: append(h.attrs, attrs...),
	}
}
