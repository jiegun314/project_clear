//go:build windows

package service

import (
	"os"
	"syscall"
	"time"
)

// sourceFileCreatedAt returns the file's creation time where the platform
// records one (Windows: CreationTime); otherwise the last-write time.
func sourceFileCreatedAt(info os.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, st.CreationTime.Nanoseconds())
	}
	return info.ModTime()
}
