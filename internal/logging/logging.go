// Package logging 提供统一的结构化日志能力：
// 控制台输出带颜色，落盘文件保持纯文本（不含 ANSI 转义）。
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	colorReset  = "\x1b[0m"
	colorGray   = "\x1b[90m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorRed    = "\x1b[31m"
	colorBold   = "\x1b[1m"
)

type output struct {
	w     io.Writer
	color bool
}

// handler 是自定义 slog.Handler，同时把同一行日志写入多个输出端。
type handler struct {
	mu     *sync.Mutex
	outs   []output
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// New 创建日志器，返回 logger 以及关闭文件句柄的函数。
func New(filePath string, debug bool) (*slog.Logger, func() error, error) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}

	h := &handler{
		mu:    &sync.Mutex{},
		level: level,
		outs: []output{
			{w: os.Stdout, color: true},
			{w: f, color: false},
		},
	}
	return slog.New(h), f.Close, nil
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Time.Format("2006-01-02 15:04:05.000"))
	b.WriteString(" [")
	paddedLevel := levelTag(r.Level) + strings.Repeat(" ", 5-len(levelTag(r.Level)))
	b.WriteString(paddedLevel)
	b.WriteString("] ")
	b.WriteString(r.Message)

	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	for _, a := range attrs {
		b.WriteString(" ")
		writeAttr(&b, h.groups, a)
	}
	b.WriteString("\n")

	line := b.String()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, o := range h.outs {
		if o.color {
			colored := strings.Replace(line, "["+paddedLevel, "["+levelColor(r.Level)+paddedLevel+colorReset, 1)
			_, _ = io.WriteString(o.w, colored)
			continue
		}
		_, _ = io.WriteString(o.w, line)
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *handler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func writeAttr(b *strings.Builder, groups []string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, sub := range a.Value.Group() {
			writeAttr(b, append(groups, a.Key), sub)
		}
		return
	}
	for _, g := range groups {
		b.WriteString(g)
		b.WriteString(".")
	}
	b.WriteString(a.Key)
	b.WriteString("=")
	b.WriteString(quoteIfNeeded(a.Value.String()))
}

func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\n\"=") {
		return strconv.Quote(s)
	}
	return s
}

func levelTag(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN "
	case l >= slog.LevelInfo:
		return "INFO "
	default:
		return "DEBUG"
	}
}

func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return colorBold + colorRed
	case l >= slog.LevelWarn:
		return colorYellow
	case l >= slog.LevelInfo:
		return colorGreen
	default:
		return colorGray
	}
}

// Fmt 用于输出启动横幅等非结构化信息（仅控制台）。
func Fmt(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
