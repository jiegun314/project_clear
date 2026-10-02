package logging

import (
	"strings"
	"testing"
	"time"
)

// The log panel reads 时:分:秒; milliseconds only add noise there. The file on
// disk keeps them, so this pins the in-memory entry format alone.
func TestEntryTimeIsToTheSecond(t *testing.T) {
	l := New("")
	l.Info("测试", "hello")

	entries := l.Recent(1)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	got := entries[0].Time
	if strings.Contains(got, ".") {
		t.Errorf("entry time %q still carries milliseconds", got)
	}
	if _, err := time.Parse("15:04:05", got); err != nil {
		t.Errorf("entry time %q is not 时:分:秒: %v", got, err)
	}
	if n := len(got); n != 8 {
		t.Errorf("entry time %q has length %d, want 8", got, n)
	}
}
