package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"project_clear/internal/config"
	"project_clear/internal/service"
	"project_clear/internal/view"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------- export

// Export asks where to save the data and writes it. An empty mode uses the
// configured default (纯数据); "clean" and "template" force one engine.
func (a *App) Export(weekCode string, mode string) (out *view.ExportResult, err error) {
	defer a.recoverFault("Export", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	effective := config.ExportMode(mode)
	if effective == "" {
		effective = c.ExportMode
	}
	ext := ".xlsm"
	if effective == config.ExportClean {
		ext = ".xlsx"
	}
	// The save dialog needs a name before the export runs; with no week named
	// this is whatever the main grid is showing.
	target := a.svc.ExportWeek(weekCode)
	if target == "" {
		return nil, fmt.Errorf("没有可导出的数据，请先导入文件")
	}
	dir := c.ExportDir
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, "Documents")
		}
	}
	dest, err := selectSavePath("导出整合数据", fmt.Sprintf("CLEAR_%s%s", target, ext), dir,
		[]string{strings.TrimPrefix(ext, ".")})
	if err != nil {
		return nil, err
	}
	if dest == "" {
		return nil, nil
	}
	a.rememberDir(filepath.Dir(dest))
	if !strings.EqualFold(filepath.Ext(dest), ext) {
		dest = strings.TrimSuffix(dest, filepath.Ext(dest)) + ext
	}
	res, err := a.svc.Export(service.ExportOptions{WeekCode: weekCode, DestPath: dest, Mode: effective}, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return &view.ExportResult{
		DestPath: dest, Mode: res.Mode, Rows: res.Rows, Cols: res.Cols,
		Comments: res.Comments, CFRows: res.CFRows, StylesUsed: res.StylesUsed,
		Dropped: res.CFRulesDropped, SizeBytes: res.SizeBytes,
		PreservedVBA: res.PreservedVBA, DurationMS: res.DurationMS,
	}, nil
}

// RevealExport opens the folder containing an exported file.
func (a *App) RevealExport(path string) {
	defer a.recoverFault("RevealExport", nil)
	if path == "" {
		return
	}
	wr.BrowserOpenURL(a.ctx, "file://"+filepath.Dir(path))
}
