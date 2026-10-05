package main

import (
	"project_clear/internal/config"
	"project_clear/internal/view"
)

// ---------------------------------------------------------------- config

// GetConfig returns the active parameters and where they live.
func (a *App) GetConfig() (out *view.ConfigView, err error) {
	defer a.recoverFault("GetConfig", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return &view.ConfigView{Config: a.cfg.Get(), Path: a.cfg.Path()}, nil
}

// SaveConfig validates and persists parameters, reporting any corrections.
func (a *App) SaveConfig(c config.Config) (out []string, err error) {
	defer a.recoverFault("SaveConfig", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	notes, err := a.cfg.Save(c)
	if notes == nil {
		// Wails turns a nil slice into null; the dialog reads this as an array.
		notes = []string{}
	}
	if err != nil {
		a.log.Error("参数", "保存失败: %v", err)
		return notes, err
	}
	if len(notes) > 0 {
		for _, n := range notes {
			a.log.Warn("参数", "%s", n)
		}
	} else {
		a.log.Success("参数", "已保存：读取列数 %d，LOC 筛选 %s，每页 %d 行，导出模式 %s",
			c.ReadColumns, c.LOCFilter, c.PageSize, c.ExportMode)
	}
	return notes, nil
}

// ResetConfig restores the shipped defaults.
func (a *App) ResetConfig() (out *view.ConfigView, err error) {
	defer a.recoverFault("ResetConfig", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	if err := a.cfg.ResetToDefault(); err != nil {
		return nil, err
	}
	a.log.Info("参数", "已恢复默认参数")
	return &view.ConfigView{Config: a.cfg.Get(), Path: a.cfg.Path()}, nil
}
