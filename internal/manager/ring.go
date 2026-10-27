package manager

import (
	"bytes"
	"sync"
	"time"
)

// LogLine 是写入内存环形缓冲的一行任务输出。
type LogLine struct {
	Time   time.Time `json:"time"`
	Stream string    `json:"stream"`
	Text   string    `json:"text"`
}

// ringBuffer 是有容量上限的日志环形缓冲，避免长时间运行占用过多内存。
type ringBuffer struct {
	mu       sync.Mutex
	lines    []LogLine
	capacity int
}

func newRing(capacity int) *ringBuffer {
	if capacity <= 0 {
		capacity = 2000
	}
	return &ringBuffer{capacity: capacity, lines: make([]LogLine, 0, capacity)}
}

func (r *ringBuffer) append(stream, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, LogLine{Time: time.Now(), Stream: stream, Text: text})
	if len(r.lines) > r.capacity {
		overflow := len(r.lines) - r.capacity
		r.lines = append(r.lines[:0], r.lines[overflow:]...)
	}
}

func (r *ringBuffer) snapshot(limit int) []LogLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > len(r.lines) {
		limit = len(r.lines)
	}
	out := make([]LogLine, limit)
	copy(out, r.lines[len(r.lines)-limit:])
	return out
}

// lineWriter 把子进程输出按行切分后写入环形缓冲。
type lineWriter struct {
	inst   *instance
	stream string

	mu      sync.Mutex
	pending []byte
}

func newLineWriter(inst *instance, stream string) *lineWriter {
	return &lineWriter{inst: inst, stream: stream}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, p...)
	for {
		idx := bytes.IndexByte(w.pending, '\n')
		if idx < 0 {
			break
		}
		line := string(w.pending[:idx])
		w.pending = w.pending[idx+1:]
		w.inst.appendLog(w.stream, trimCR(line))
	}
	return len(p), nil
}

// Flush 输出最后一段没有换行符的内容。
func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	w.inst.appendLog(w.stream, trimCR(string(w.pending)))
	w.pending = nil
}

func trimCR(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
