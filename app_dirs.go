package main

import (
	"os"
	"path/filepath"

	"project_clear/internal/config"
)

// ---------------------------------------------------------------- app info

// startDir picks the folder a chooser should open.
//
// A remembered folder is only used while it is still there. Handing the system
// dialog a folder that has since been deleted or moved gets the user a "folder
// does not exist" error and no way past it, so the program's own folder is used
// instead — somewhere they can see, and where their data lives — with the home
// folder as the last resort.
func startDir(remembered string, exists func(string) bool, candidates ...string) string {
	if remembered != "" && exists(remembered) {
		return remembered
	}
	for _, c := range candidates {
		if c != "" && exists(c) {
			return c
		}
	}
	return ""
}

// isDir reports whether a path is an existing folder.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// lastDir is where the chooser should open; the last folder used is the most
// useful default for a weekly batch job.
func (a *App) lastDir() string {
	home := ""
	if h, err := os.UserHomeDir(); err == nil {
		home = filepath.Join(h, "Documents")
	}
	// A build run from `go run` sits in a throwaway folder: ProgramDir falls back
	// to the working directory there, which is still a folder that exists.
	program, err := config.ProgramDir()
	if err != nil {
		program = ""
	}
	return startDir(a.cfg.Get().ExportDir, isDir, program, home)
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
