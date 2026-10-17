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

// GetStatus returns the current state for the strip and the status bar.
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
	ws, err := a.svc.Workspace()
	if err != nil {
		return nil, err
	}
	st.ActiveWeek = ws.Week
	st.Unintegrated = ws.Unintegrated
	st.CanDiscardWeek = ws.Unintegrated
	if ws.Draft != nil {
		st.HasDraft = true
		st.DraftID = ws.Draft.ID
		st.DraftWeek = ws.Draft.WeekCode
		st.DraftRows = ws.Draft.RowCount
		st.DraftFiles = ws.Draft.FileCount
		st.DraftFail = ws.Draft.FailedCount
	}
	if ws.Global != nil {
		st.HasGlobal = true
		st.GlobalID = ws.Global.ID
		st.GlobalWeek = ws.Global.WeekCode
		st.GlobalRows = ws.Global.RowCount
		st.GlobalFiles = ws.Global.FileCount
		st.GlobalVersion = ws.Global.Version
		st.GlobalAt = ws.Global.ImportedAt
	}
	if ws.Local != nil {
		st.HasLocal = true
		st.LocalID = ws.Local.ID
		st.LocalWeek = ws.Local.WeekCode
		st.LocalRows = ws.Local.RowCount
		st.LocalFiles = ws.Local.FileCount
		st.LocalVersion = ws.Local.Version
		st.LocalAt = ws.Local.ImportedAt
		if files, err := a.db.DatasetFiles(ws.Local.ID); err == nil {
			for _, f := range files {
				if f.Status == "ok" {
					st.LocalFileName = f.Name
					break
				}
			}
		}
	}
	if ws.Integration != nil {
		st.HasIntegration = true
		st.IntegrationID = ws.Integration.MergedID
		st.IntegrationWeek = ws.Integration.WeekCode
		st.IntegrationAt = ws.Integration.IntegratedAt
		st.IntegrationExpired = ws.Expired
		if merged, err := a.db.Dataset(ws.Integration.MergedID); err == nil && merged != nil {
			st.IntegrationRows = merged.RowCount
		}
	}
	// The bottom status bar's 已整合 count is integration weeks, not the history
	// list: history now also contains weeks whose imports are complete but not
	// integrated yet.
	integrations, err := a.db.ListIntegrations()
	if err != nil {
		return nil, err
	}
	st.ArchivedWeeks = len(integrations)
	for _, in := range integrations {
		if merged, err := a.db.Dataset(in.MergedID); err == nil && merged != nil {
			st.ArchivedRows += merged.RowCount
		}
	}
	if len(a.log.Recent(1)) > 0 {
		last := a.log.Recent(1)[0]
		st.LastAction = fmt.Sprintf("%s %s", last.Time, last.Message)
	}
	return st, nil
}
