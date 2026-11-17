//go:build darwin

package service

import (
	"os"
	"syscall"
	"time"
)

// sourceFileCreatedAt returns the file's creation time where the platform
// records one (macOS: birthtime); otherwise the last-write time.
func sourceFileCreatedAt(info os.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		ts := st.Birthtimespec
		if ts.Sec != 0 || ts.Nsec != 0 {
			return time.Unix(ts.Sec, ts.Nsec)
		}
	}
	return info.ModTime()
}
