package main

import (
	"fmt"
	"path/filepath"

	"project_clear/internal/master"
)

// GetMasterOverview returns the counters for the 主数据 page.
func (a *App) GetMasterOverview() (out master.Overview, err error) {
	defer a.recoverFault("GetMasterOverview", &err)
	if err := a.ready(); err != nil {
		return out, err
	}
	return a.svc.MasterOverview()
}

// QueryMasterData returns one page of a master-data table.
func (a *App) QueryMasterData(q master.Query) (out master.Page, err error) {
	defer a.recoverFault("QueryMasterData", &err)
	if err := a.ready(); err != nil {
		return out, err
	}
	return a.svc.MasterPage(q)
}

// SaveMasterRow creates or updates one mapping row.
func (a *App) SaveMasterRow(in master.Input) (err error) {
	defer a.recoverFault("SaveMasterRow", &err)
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SaveMasterRow(in)
}

// DeleteMasterRow removes one mapping row.
func (a *App) DeleteMasterRow(table, key string) (err error) {
	defer a.recoverFault("DeleteMasterRow", &err)
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.DeleteMasterRow(table, key)
}

// ImportMasterWorkbook asks for the matching workbook and replaces the current
// master data with its contents. The import is all-or-nothing.
func (a *App) ImportMasterWorkbook() (out *master.ImportResult, err error) {
	defer a.recoverFault("ImportMasterWorkbook", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	paths, err := selectOneFile("选择代码与分类匹配工作簿", a.lastDir(), []string{"xlsx", "xlsm"})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > 1 {
		return nil, fmt.Errorf("请选择一个匹配工作簿")
	}
	a.rememberDir(filepath.Dir(paths[0]))
	return a.svc.ImportMasterWorkbook(paths[0])
}
