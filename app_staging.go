package main

import (
	"fmt"
	"path/filepath"

	"project_clear/internal/service"
	"project_clear/internal/view"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------- imports

// ImportFolder asks for a folder and writes every workbook inside it into a
// fresh Global draft.
func (a *App) ImportFolder() (out *service.ImportResult, err error) {
	defer a.recoverFault("ImportFolder", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	picked, err := selectOneFolder("选择包含 MPS 源文件的文件夹", a.lastDir())
	if err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, nil
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

// AddFiles asks for one or more workbooks and adds them to the Global draft.
func (a *App) AddFiles() (out *service.ImportResult, err error) {
	defer a.recoverFault("AddFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	paths, err := selectManyFiles("选择 MPS 源文件（可多选）", a.lastDir(),
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
	res, err := a.svc.AddFiles(paths, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// FinishGlobalMPS completes the Global draft. confirmFailures is the caller's
// acknowledgement that the failed workbooks listed in the 已导入文件 window are
// acceptable.
func (a *App) FinishGlobalMPS(confirmFailures bool) (out *service.FinishResult, err error) {
	defer a.recoverFault("FinishGlobalMPS", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, err := a.svc.FinishGlobal(confirmFailures)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// ImportLocalMPS imports the 本地 MPS workbook as a complete Local dataset.
func (a *App) ImportLocalMPS() (out *service.ImportResult, err error) {
	defer a.recoverFault("ImportLocalMPS", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	paths, err := selectOneFile("选择本地 MPS 文件（Month End）", a.lastDir(),
		[]string{"xlsx", "xlsm"})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	a.rememberDir(filepath.Dir(paths[0]))
	res, err := a.svc.ImportLocal(paths[0], a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// ---------------------------------------------------------------- integrate

// Commit combines the current Global and Local datasets into the week's
// integration.
func (a *App) Commit(weekLimit int) (out *view.CommitResult, err error) {
	defer a.recoverFault("Commit", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, had, wasExpired, err := a.svc.Commit(weekLimit)
	if err != nil {
		return nil, err
	}
	return &view.CommitResult{
		WeekCode: res.Merged.WeekCode, WeekStart: res.Merged.WeekStart,
		MergedID: res.Merged.ID, GlobalID: res.Global.ID, LocalID: res.Local.ID,
		MergedRows: res.Merged.RowCount, GlobalRows: res.Global.RowCount, LocalRows: res.Local.RowCount,
		MergedVer: res.Merged.Version, GlobalVer: res.Global.Version, LocalVer: res.Local.Version,
		FileCount: res.Merged.FileCount, CommittedAt: res.Integration.IntegratedAt,
		Overwrote: had, WasExpired: wasExpired,
		RequestedWeeks: res.RequestedWeeks, MergedWeeks: res.MergedWeeks, Warnings: res.Warnings,
	}, nil
}

// ---------------------------------------------------------------- workspace

// GetFiles lists the workbooks of the current draft/Global and Local imports.
func (a *App) GetFiles() (out *view.FilesView, err error) {
	defer a.recoverFault("GetFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	out = view.NewFilesView()
	ws, err := a.svc.Workspace()
	if err != nil {
		return nil, err
	}
	files, err := a.svc.Files()
	if err != nil {
		return nil, err
	}
	out.Files = files
	out.HasDraft = ws.Draft != nil
	if out.Files == nil {
		out.Files = []service.FileWithDataset{}
	}
	global := ws.Draft
	if global == nil {
		global = ws.Global
	}
	if global != nil {
		out.FileCount += global.FileCount
		out.RowCount += global.RowCount
		out.FailedCount += global.FailedCount
	}
	if len(out.Files) == 0 && out.FileCount > 0 {
		out.Note = "该周的数据由旧版本迁移而来，未保留逐文件清单；文件数、行数来自数据集记录。"
	}
	return out, nil
}

// ---------------------------------------------------------------- discard

// DiscardDraft removes the open Global draft. Completed datasets and history
// are untouched.
func (a *App) DiscardDraft() (out *service.ImportResult, err error) {
	defer a.recoverFault("DiscardDraft", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.DiscardDraft()
}

// DiscardWeekImports removes the active week's un-integrated imports. The
// integration (history) is never touched.
func (a *App) DiscardWeekImports(weekCode string) (out *view.DiscardWeekResult, err error) {
	defer a.recoverFault("DiscardWeekImports", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	if weekCode == "" {
		return nil, fmt.Errorf("没有要丢弃的导入")
	}
	n, err := a.svc.DiscardWeekImports(weekCode)
	if err != nil {
		return nil, err
	}
	return &view.DiscardWeekResult{WeekCode: weekCode, Datasets: n}, nil
}
