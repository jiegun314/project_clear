package main

import (
	"fmt"
	"path/filepath"
	"runtime"

	"project_clear/internal/store"
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
		// The two inputs live side by side until 整合; the grid shows one of them.
		st.StagedRows = sum.Rows(sum.DisplayDataset())
		st.HasGlobal = sum.Has(store.DatasetGlobal)
		st.HasLocal = sum.Has(store.DatasetLocal)
		st.GlobalRows = sum.Rows(store.DatasetGlobal)
		st.LocalRows = sum.Rows(store.DatasetLocal)
		st.LastDataset = sum.DisplayDataset()
	}
	list, err := a.svc.Archive()
	if err != nil {
		return nil, err
	}
	// A week holds three registry rows (merged, global, local); the status bar
	// counts weeks, and the row total is the merged results it can show.
	seenWeeks := map[string]bool{}
	for _, e := range list {
		if !seenWeeks[e.WeekCode] {
			seenWeeks[e.WeekCode] = true
			st.ArchivedWeeks++
		}
		if e.Dataset == store.DatasetMerged {
			st.ArchivedRows += e.RowCount
		}
	}
	if !sum.HasStaging && len(list) > 0 {
		// Report the newest week's two inputs as well, so the status payload
		// answers "what is in the library" the same way for both states.
		newest := list[0].WeekCode
		for _, e := range list {
			if e.WeekCode != newest {
				continue
			}
			switch e.Dataset {
			case store.DatasetGlobal:
				st.HasGlobal, st.GlobalRows = true, e.RowCount
			case store.DatasetLocal:
				st.HasLocal, st.LocalRows = true, e.RowCount
			}
		}
	}
	if len(a.log.Recent(1)) > 0 {
		last := a.log.Recent(1)[0]
		st.LastAction = fmt.Sprintf("%s %s", last.Time, last.Message)
	}
	return st, nil
}
