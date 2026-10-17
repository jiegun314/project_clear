package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"project_clear/internal/config"
	"project_clear/internal/mps"
	"project_clear/internal/service"
	"project_clear/internal/view"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------- export

// Export asks where to save one dataset and writes it as the single-sheet
// report. An empty mode uses the configured default (纯数据); "clean" and
// "template" force one engine. The dataset is what the history window picked:
// global, local or merged (empty means merged).
func (a *App) Export(weekCode string, dataset string, mode string) (out *view.ExportResult, err error) {
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
	// The same folder the other choosers open: the last one used while it still
	// exists, otherwise the program's folder. Sharing lastDir also means a folder
	// that has been deleted cannot strand this dialog either.
	dest, err := selectSavePath("导出整合数据", fmt.Sprintf("CLEAR_%s%s", target, ext), a.lastDir(),
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
	res, err := a.svc.Export(service.ExportOptions{WeekCode: weekCode, Dataset: dataset, DestPath: dest, Mode: effective}, a.progress)
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
		Sheets: res.Sheets,
	}, nil
}

// ExportBundle asks where to save and writes the three-table workbook: MPS (the
// merged result), Local and Global, one sheet each. The save dialog offers the
// name the business uses for that week.
func (a *App) ExportBundle(weekCode string) (out *view.ExportResult, err error) {
	defer a.recoverFault("ExportBundle", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	target := a.svc.ExportWeek(weekCode)
	if target == "" {
		return nil, fmt.Errorf("没有可导出的数据，请先导入文件")
	}
	w, err := mps.ParseWeekCode(target)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("BW_CN_MPS Week%d %d.xlsx", w.WeekNo, w.FullYear())
	dest, err := selectSavePath("导出三表文件（MPS / Local / Global）", name, a.lastDir(),
		[]string{"xlsx"})
	if err != nil {
		return nil, err
	}
	if dest == "" {
		return nil, nil
	}
	a.rememberDir(filepath.Dir(dest))
	if !strings.EqualFold(filepath.Ext(dest), ".xlsx") {
		dest = strings.TrimSuffix(dest, filepath.Ext(dest)) + ".xlsx"
	}
	res, err := a.svc.ExportBundle(service.BundleExportOptions{WeekCode: weekCode, DestPath: dest}, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return &view.ExportResult{
		DestPath: dest, Mode: res.Mode, Rows: res.Rows, Cols: res.Cols,
		SizeBytes: res.SizeBytes, DurationMS: res.DurationMS, Sheets: res.Sheets,
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
