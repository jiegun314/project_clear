//go:build !darwin && !windows

package service

import (
	"os"
	"time"
)

// sourceFileCreatedAt falls back to the last-write time on platforms where Go
// does not expose a portable file creation time.
func sourceFileCreatedAt(info os.FileInfo) time.Time {
	return info.ModTime()
}
