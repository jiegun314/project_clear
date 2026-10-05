package main

import (
	"fmt"

	"project_clear/internal/mps"
	"project_clear/internal/store"
	"project_clear/internal/view"
)

// ---------------------------------------------------------------- data

// QueryData returns one page of the merged data.
func (a *App) QueryData(q view.GridQuery) (out *store.GridResult, err error) {
	defer a.recoverFault("QueryData", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	if q.PageSize <= 0 {
		q.PageSize = c.PageSize
	}
	return a.svc.Query(store.Query{
		Source: q.Source, Page: q.Page, PageSize: q.PageSize,
		Search: q.Search, SortField: q.SortField, SortDesc: q.SortDesc, Filters: q.Filters,
	})
}

// GetGridHeader returns the column layout for a source.
func (a *App) GetGridHeader(source string) (out *view.GridHeader, err error) {
	defer a.recoverFault("GetGridHeader", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	out = view.NewGridHeader(source)
	// The week-code check stays here so a crafted request is rejected before it
	// reaches the store; resolving the source itself belongs to the service.
	if source != "" && !store.ValidWeekCode(source) {
		return nil, fmt.Errorf("非法周码 %q", source)
	}
	src, err := a.svc.GridSource(source)
	if err != nil {
		return nil, err
	}
	out.HasStaging = src.HasStaging
	out.WeekCode = src.WeekCode
	out.WeekStart = src.WeekStart
	if src.IndexNames != nil {
		out.IndexNames = src.IndexNames
	}
	for _, c := range src.WeekCodes {
		w, err := mps.ParseWeekCode(c)
		if err != nil {
			return nil, err
		}
		out.Weeks = append(out.Weeks, w)
	}
	return out, nil
}

// GetArchive lists committed weeks.
func (a *App) GetArchive() (out []store.ArchiveEntry, err error) {
	defer a.recoverFault("GetArchive", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Archive()
}

// GetYears lists the years that have committed weeks.
func (a *App) GetYears() (out []int, err error) {
	defer a.recoverFault("GetYears", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Years()
}

// GetWeeks lists committed week numbers for a year.
func (a *App) GetWeeks(year int) (out []int, err error) {
	defer a.recoverFault("GetWeeks", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Weeks(year)
}
