package main

import (
	"path/filepath"

	"project_clear/internal/service"
	"project_clear/internal/view"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------- import

// ImportFolder asks for a folder and imports every workbook inside it.
func (a *App) ImportFolder() (out *service.ImportResult, err error) {
	defer a.recoverFault("ImportFolder", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	picked, err := selectPaths("选择包含 MPS 源文件的文件夹", a.lastDir(), false, false, nil)
	if err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, nil // cancelled
	}
	dir := picked[0]
	a.rememberDir(dir)
	res, err := a.svc.ImportFolder(dir, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// AddFiles asks for one or more workbooks and stages them.
func (a *App) AddFiles() (out *service.ImportResult, err error) {
	defer a.recoverFault("AddFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	paths, err := selectPaths("选择 MPS 源文件（可多选）", a.lastDir(), true, true,
		[]string{"xlsm", "xlsx"})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) == 1 {
		a.rememberDir(filepath.Dir(paths[0]))
	}
	// A file whose name is already in the list is replaced, not merged: the
	// service does that without asking, and reports the names it overwrote.
	res, err := a.svc.AddFiles(paths, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// ---------------------------------------------------------------- commit

// Commit promotes the staged data into its permanent weekly table.
func (a *App) Commit() (out *view.CommitResult, err error) {
	defer a.recoverFault("Commit", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	entry, overwrote, err := a.svc.Commit()
	if err != nil {
		return nil, err
	}
	return &view.CommitResult{
		WeekCode: entry.WeekCode, WeekStart: entry.WeekStart, TableName: entry.TableName,
		RowCount: entry.RowCount, FileCount: entry.FileCount, CommittedAt: entry.CommittedAt,
		Overwrote: overwrote,
	}, nil
}

// ------------------------------------------------------- staging file list

// GetStagingFiles lists the work set behind the toolbar's 已导入文件 button: the
// files that were imported (or added), and — after 整合 — the same list as the
// record of that import. It stays until 清空 or the next import.
func (a *App) GetStagingFiles() (out *view.StagingFilesView, err error) {
	defer a.recoverFault("GetStagingFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	out = view.NewStagingFilesView()
	files, _, state, err := a.svc.BatchFiles()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return out, nil
	}
	out.HasStaging = state == "staging"
	out.BatchState = state
	out.Files = files
	for _, f := range files {
		if f.Status == "ok" {
			out.FileCount++
			out.RowCount += f.RowsKept
		} else {
			out.Failed++
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- clear

// ClearStaging drops every imported-but-unsaved row: the staging area and the
// export template that came with it. Weeks already integrated are untouched.
func (a *App) ClearStaging() (out *view.ClearStagingResult, err error) {
	defer a.recoverFault("ClearStaging", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	sum, err := a.svc.StagingSummary()
	if err != nil {
		return nil, err
	}
	// 清空 always runs: with no staging left it only ends the 已导入文件 list
	// (the record of the batch that was just integrated).
	rows, err := a.svc.ClearStaging()
	if err != nil {
		return nil, err
	}
	return &view.ClearStagingResult{WeekCode: sum.WeekCode, Files: sum.FileOK, Rows: rows}, nil
}
