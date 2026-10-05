package main

import (
	"os"
	"path/filepath"
)

// ---------------------------------------------------------------- app info

// lastDir is where the chooser should open; the last folder used is the most
// useful default for a weekly batch job.
func (a *App) lastDir() string {
	c := a.cfg.Get()
	if c.ExportDir != "" {
		return c.ExportDir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Documents")
	}
	return ""
}

func (a *App) rememberDir(dir string) {
	if dir == "" {
		return
	}
	c := a.cfg.Get()
	if c.ExportDir == dir {
		return
	}
	c.ExportDir = dir
	if _, err := a.cfg.Save(c); err != nil {
		a.log.Warn("参数", "保存目录失败: %v", err)
	}
}
