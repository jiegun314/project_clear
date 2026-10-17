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
// report. An empty mode uses the configured default (纯数据).
func (a *App) Export(datasetID int64, mode string) (out *view.ExportResult, err error) {
	defer a.recoverFault("Export", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	src, err := a.svc.GridSource(datasetID)
	if err != nil {
		return nil, err
	}
	if src.Dataset == nil {
		return nil, fmt.Errorf("没有可导出的数据")
	}
	d := src.Dataset
	c := a.cfg.Get()
	effective := config.ExportMode(mode)
	if effective == "" {
		effective = c.ExportMode
	}
	ext := ".xlsm"
	if effective == config.ExportClean {
		ext = ".xlsx"
	}
	name := fmt.Sprintf("CLEAR_%s_%s_v%d%s", d.WeekCode, d.Kind, d.Version, ext)
	dest, err := selectSavePath("导出数据", name, a.lastDir(),
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
	res, err := a.svc.Export(service.ExportOptions{DatasetID: datasetID, DestPath: dest, Mode: effective}, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return &view.ExportResult{
		DestPath: dest, DatasetID: datasetID, WeekCode: d.WeekCode,
		Mode: res.Mode, Rows: res.Rows, Cols: res.Cols,
		Comments: res.Comments, CFRows: res.CFRows, StylesUsed: res.StylesUsed,
		Dropped: res.CFRulesDropped, SizeBytes: res.SizeBytes,
		PreservedVBA: res.PreservedVBA, DurationMS: res.DurationMS,
		Sheets: res.Sheets, Expired: src.Expired,
	}, nil
}

// ExportBundle asks where to save and writes the integrated week's three-table
// workbook: MPS (merged), Local and Global.
func (a *App) ExportBundle(weekCode string) (out *view.ExportResult, err error) {
	defer a.recoverFault("ExportBundle", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	target := a.svc.ExportWeek(weekCode)
	if target == "" {
		return nil, fmt.Errorf("没有可导出的数据，请先导入并整合")
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
	res, err := a.svc.ExportBundle(service.BundleExportOptions{WeekCode: target, DestPath: dest}, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return &view.ExportResult{
		DestPath: dest, WeekCode: target, Mode: res.Mode, Rows: res.Rows, Cols: res.Cols,
		SizeBytes: res.SizeBytes, DurationMS: res.DurationMS, Sheets: res.Sheets,
		Expired: res.Expired,
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
