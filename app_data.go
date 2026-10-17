package main

import (
	"project_clear/internal/service"
	"project_clear/internal/store"
	"project_clear/internal/view"
)

// ---------------------------------------------------------------- data grid

// QueryData returns one page of a dataset.
func (a *App) QueryData(q view.GridQuery) (out *store.GridResult, err error) {
	defer a.recoverFault("QueryData", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	if q.PageSize <= 0 || q.PageSize > 5000 {
		q.PageSize = c.PageSize
	}
	return a.svc.Query(store.DatasetQuery{
		DatasetID: q.DatasetID, Page: q.Page, PageSize: q.PageSize,
		Search: q.Search, SortField: q.SortField, SortDesc: q.SortDesc, Filters: q.Filters,
	})
}

// GetGridHeader returns the column layout of one dataset.
func (a *App) GetGridHeader(datasetID int64) (out *view.GridHeader, err error) {
	defer a.recoverFault("GetGridHeader", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	src, err := a.svc.GridSource(datasetID)
	if err != nil {
		return nil, err
	}
	if src.Dataset == nil {
		return view.NewGridHeader(), nil
	}
	d := src.Dataset
	return (&view.GridHeader{
		DatasetID: d.ID, Kind: d.Kind, Version: d.Version,
		WeekCode: d.WeekCode, WeekStart: d.WeekStart,
		IndexNames: d.IndexNames, Weeks: src.WeekCodes,
		Total: d.RowCount, Expired: src.Expired,
	}).NonNil(), nil
}

// ---------------------------------------------------------------- archive

// GetArchive lists the integrated weeks as their three dataset entries.
func (a *App) GetArchive() (out []service.ArchiveEntry, err error) {
	defer a.recoverFault("GetArchive", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Archive()
}

// GetYears lists the years that have an integrated week.
func (a *App) GetYears() (out []int, err error) {
	defer a.recoverFault("GetYears", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Years()
}

// GetWeeks lists the ISO weeks of a year that have an integration.
func (a *App) GetWeeks(year int) (out []int, err error) {
	defer a.recoverFault("GetWeeks", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Weeks(year)
}
