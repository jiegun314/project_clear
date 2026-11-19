package service

import (
	"fmt"
	"strings"

	"project_clear/internal/master"
)

// MasterOverview returns the counters for the 主数据 page.
func (s *Service) MasterOverview() (master.Overview, error) {
	return s.db.MasterOverview()
}

// MasterPage returns one page of one master-data table.
func (s *Service) MasterPage(q master.Query) (master.Page, error) {
	return s.db.MasterPage(q)
}

// SaveMasterRow creates or updates one mapping row.
func (s *Service) SaveMasterRow(in master.Input) error {
	if err := s.db.SaveMasterRow(in); err != nil {
		return err
	}
	if s.log != nil {
		s.log.Info("主数据", "保存 %s：%s", in.Table,
			strings.TrimSpace(in.ItemCode+in.BaseCode+in.SapCode+in.Material))
	}
	return nil
}

// DeleteMasterRow removes one mapping row. The store refuses a delete that would
// leave referencing rows behind.
func (s *Service) DeleteMasterRow(table, key string) error {
	if err := s.db.DeleteMasterRow(table, key); err != nil {
		return err
	}
	if s.log != nil {
		s.log.Info("主数据", "删除 %s：%s", table, key)
	}
	return nil
}

// ImportMasterWorkbook reads the supplied workbook and replaces all three
// master-data tables in one transaction. Unclassified rows and orphan Item→Base
// references are reported, never treated as import failures.
func (s *Service) ImportMasterWorkbook(path string) (*master.ImportResult, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("未选择主数据工作簿")
	}
	w, err := master.ReadWorkbook(path)
	if err != nil {
		return nil, err
	}
	if len(w.Items) == 0 && len(w.Bases) == 0 && len(w.Categories) == 0 {
		return nil, fmt.Errorf("工作簿中没有可导入的匹配数据")
	}
	if err := s.db.ReplaceMasterData(w); err != nil {
		return nil, fmt.Errorf("写入主数据失败: %w", err)
	}
	ov, err := s.db.MasterOverview()
	if err != nil {
		return nil, err
	}
	out := &master.ImportResult{
		Path:          path,
		Name:          w.Name,
		Items:         len(w.Items),
		Bases:         len(w.Bases),
		Categories:    len(w.Categories),
		DuplicateRows: w.DuplicateRows,
		Orphans:       ov.OrphanCount,
		Uncategorized: ov.Uncategorized,
		Warnings:      w.Warnings,
		ImportedAt:    ov.LastImportAt,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	if s.log != nil {
		s.log.Success("主数据", "导入 %s：Item %d、Base %d、SAP 分类 %d；未分类 %d、孤儿 Base %d",
			w.Name, out.Items, out.Bases, out.Categories, out.Uncategorized, out.Orphans)
		for _, warning := range w.Warnings {
			s.log.Warn("主数据", "%s", warning)
		}
	}
	return out, nil
}

// ImportPriceWorkbook reads the supplied price workbook and replaces the local
// product-price table in one transaction. Rows without a price are kept as
// unpriced; only a non-numeric price or a blank Material is skipped.
func (s *Service) ImportPriceWorkbook(path string) (*master.PriceImportResult, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("未选择价格工作簿")
	}
	w, err := master.ReadPriceWorkbook(path)
	if err != nil {
		return nil, err
	}
	if len(w.Prices) == 0 {
		return nil, fmt.Errorf("工作簿中没有可导入的价格数据")
	}
	if err := s.db.ReplacePriceData(w); err != nil {
		return nil, fmt.Errorf("写入价格主数据失败: %w", err)
	}
	ov, err := s.db.MasterOverview()
	if err != nil {
		return nil, err
	}
	out := &master.PriceImportResult{
		Path:          path,
		Name:          w.Name,
		Prices:        len(w.Prices),
		DuplicateRows: w.DuplicateRows,
		MissingCosts:  w.MissingCosts,
		SkippedRows:   w.SkippedRows,
		Warnings:      w.Warnings,
		ImportedAt:    ov.LastPriceImportAt,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	if s.log != nil {
		s.log.Success("主数据", "导入价格 %s：%d 条，未定价 %d 条", w.Name, out.Prices, out.MissingCosts)
		for _, warning := range w.Warnings {
			s.log.Warn("主数据", "%s", warning)
		}
	}
	return out, nil
}
