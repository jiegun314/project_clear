// Package logging keeps an in-memory ring of recent log entries for the UI and
// mirrors everything to a daily file on disk.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level classifies a log entry.
type Level string

const (
	LevelInfo    Level = "info"
	LevelSuccess Level = "success"
	LevelWarn    Level = "warn"
	LevelError   Level = "error"
)

// Entry is a single log line handed to the frontend.
type Entry struct {
	Seq     int64  `json:"seq"`
	Time    string `json:"time"`
	Level   Level  `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

const maxEntries = 2000

// Logger is safe for concurrent use.
type Logger struct {
	mu      sync.RWMutex
	entries []Entry
	seq     int64
	dir     string
	subs    map[chan Entry]struct{}
}

// New creates a logger that also writes into dir (created on demand).
func New(dir string) *Logger {
	l := &Logger{dir: dir, subs: map[chan Entry]struct{}{}}
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return l
}

func (l *Logger) write(level Level, source, format string, args ...any) {
	e := Entry{
		Seq: l.next(),
		// The log panel shows time to the second; the file keeps milliseconds.
		Time:   time.Now().Format("15:04:05"),
		Level:  level,
		Source: source,
		// Wrapped errors arrive in English; the panel is Chinese, so the text
		// is localised once here and both sinks show the same wording.
		Message: localize(fmt.Sprintf(format, args...)),
	}
	l.mu.Lock()
	l.entries = append(l.entries, e)
	if len(l.entries) > maxEntries {
		l.entries = l.entries[len(l.entries)-maxEntries:]
	}
	subs := make([]chan Entry, 0, len(l.subs))
	for c := range l.subs {
		subs = append(subs, c)
	}
	l.mu.Unlock()

	for _, c := range subs {
		select {
		case c <- e:
		default: // slow consumer: drop rather than block the importer
		}
	}
	l.toFile(e)
}

func (l *Logger) next() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	return l.seq
}

func (l *Logger) toFile(e Entry) {
	if l.dir == "" {
		return
	}
	name := filepath.Join(l.dir, "clear-"+time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [%s] %-7s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), e.Level, e.Source, e.Message)
}

// Info records a neutral message.
func (l *Logger) Info(source, format string, args ...any) {
	l.write(LevelInfo, source, format, args...)
}

// Success records a completed operation.
func (l *Logger) Success(source, format string, args ...any) {
	l.write(LevelSuccess, source, format, args...)
}

// Warn records a recoverable problem, such as a skipped file.
func (l *Logger) Warn(source, format string, args ...any) {
	l.write(LevelWarn, source, format, args...)
}

// Error records a failure.
func (l *Logger) Error(source, format string, args ...any) {
	l.write(LevelError, source, format, args...)
}

// Recent returns the newest entries first, capped at limit.
func (l *Logger) Recent(limit int) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}
	out := make([]Entry, 0, limit)
	for i := len(l.entries) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, l.entries[i])
	}
	return out
}

// Subscribe returns a channel that receives future entries, plus a cancel
// function the caller must invoke.
func (l *Logger) Subscribe() (<-chan Entry, func()) {
	c := make(chan Entry, 256)
	l.mu.Lock()
	l.subs[c] = struct{}{}
	l.mu.Unlock()
	return c, func() {
		l.mu.Lock()
		delete(l.subs, c)
		l.mu.Unlock()
	}
}

// Clear drops the in-memory buffer (the on-disk log is kept).
func (l *Logger) Clear() {
	l.mu.Lock()
	l.entries = nil
	l.mu.Unlock()
}
