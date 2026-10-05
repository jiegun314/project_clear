package main

import (
	"fmt"
	"path/filepath"
	"runtime"

	"project_clear/internal/view"
)

// GetAppInfo returns the About-window payload.
func (a *App) GetAppInfo() view.AppInfo {
	defer a.recoverFault("GetAppInfo", nil)
	info := view.AppInfo{
		Name: AppName, FullName: AppFull, Version: AppVersion,
		GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	if a.cfg != nil {
		info.ConfigPath = a.cfg.Path()
	}
	if a.data != "" {
		info.DataDir = a.data
		info.Database = filepath.Join(a.data, "clear.db")
	}
	return info
}

// ---------------------------------------------------------------- status

// GetStatus returns the current state for the status bar.
func (a *App) GetStatus() (out *view.Status, err error) {
	defer a.recoverFault("GetStatus", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	st := &view.Status{
		ReadColumns: c.ReadColumns, LOCFilter: c.LOCFilter, PageSize: c.PageSize,
		HeaderDisplay: string(c.HeaderDisplay), ExportMode: string(c.ExportMode),
		Database: filepath.Join(a.data, "clear.db"),
	}
	sum, err := a.svc.StagingSummary()
	if err != nil {
		return nil, err
	}
	if sum.HasStaging {
		st.HasStaging = true
		st.WeekCode = sum.WeekCode
		st.WeekStart = sum.WeekStart
		st.StagedRows = sum.RowKept
	}
	list, err := a.svc.Archive()
	if err != nil {
		return nil, err
	}
	st.ArchivedWeeks = len(list)
	for _, e := range list {
		st.ArchivedRows += e.RowCount
	}
	if len(a.log.Recent(1)) > 0 {
		last := a.log.Recent(1)[0]
		st.LastAction = fmt.Sprintf("%s %s", last.Time, last.Message)
	}
	return st, nil
}
