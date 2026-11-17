package service

import (
	"os"
	"time"
)

// fileCreatedAt resolves a workbook path to its source creation time. A file
// that cannot be stat'ed gets the zero time, which makes it lose a duplicate
// comparison against a readable file.
func fileCreatedAt(path string) time.Time {
	if path == "" {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return sourceFileCreatedAt(info)
}
