// Package master defines the code-matching master data and reads its workbook.
//
// The master data is deliberately separate from the weekly datasets: it is
// maintained on this machine, overwritten in place, and not versioned by week.
// A historical week keeps the codes it was imported with, so changing a mapping
// here never rewrites old data.
package master

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Table names used by the store, API and UI.
const (
	TableItem     = "item"
	TableBase     = "base"
	TableCategory = "category"
	TablePrice    = "price"
)

// ValidTable reports whether table is one of the master-data tables.
func ValidTable(table string) bool {
	switch table {
	case TableItem, TableBase, TableCategory, TablePrice:
		return true
	}
	return false
}

// ItemEntry is one row of Item Code -> Base Code.
type ItemEntry struct {
	ItemCode  string
	BaseCode  string
	SourceRow int
}

// BaseEntry is one row of Base Code -> Manu/SAP code.
type BaseEntry struct {
	BaseCode  string
	ManuCode  string
	SapCode   string
	SourceRow int
}

// CategoryEntry is one row of SAP Code -> Category.
type CategoryEntry struct {
	SapCode   string
	Category  string
	SourceRow int
}

// PriceEntry is one Material -> Standard Cost row. A nil StandardCost means the
// source row had no price; it is kept so the missing price is visible instead of
// silently disappearing.
type PriceEntry struct {
	Material     string
	StandardCost *float64
	SourceRow    int
}

// PriceWorkbook is the validated, de-duplicated content of one price file.
type PriceWorkbook struct {
	Path   string
	Name   string
	Prices []PriceEntry

	// DuplicateRows counts repeated Material keys that were collapsed.
	DuplicateRows int
	// MissingCosts counts rows whose Standard_Cost cell was blank.
	MissingCosts int
	// SkippedRows counts rows rejected for a non-numeric price or a blank key.
	SkippedRows int
	Warnings    []string
}

// Workbook is the validated, de-duplicated content of one master-data file.
type Workbook struct {
	Path       string
	Name       string
	Items      []ItemEntry
	Bases      []BaseEntry
	Categories []CategoryEntry

	// DuplicateRows counts exact duplicate keys that were collapsed.
	DuplicateRows int
	// Warnings records recoverable source problems, such as a duplicate key with
	// conflicting values. Import is not blocked by them.
	Warnings []string
}

// Row is one row returned to the UI. Only the fields for its table are set.
type Row struct {
	Table         string   `json:"table"`
	Key           string   `json:"key"`
	ItemCode      string   `json:"itemCode,omitempty"`
	BaseCode      string   `json:"baseCode,omitempty"`
	ManuCode      string   `json:"manuCode,omitempty"`
	SapCode       string   `json:"sapCode,omitempty"`
	Category      string   `json:"category,omitempty"`
	Material      string   `json:"material,omitempty"`
	StandardCost  *float64 `json:"standardCost,omitempty"`
	Uncategorized bool     `json:"uncategorized,omitempty"`
	Orphan        bool     `json:"orphan,omitempty"`
	Unused        bool     `json:"unused,omitempty"`
	RefCount      int      `json:"refCount,omitempty"`
	UpdatedAt     string   `json:"updatedAt,omitempty"`
}

// Query is the server-side page request for one master table.
type Query struct {
	Table    string `json:"table"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Search   string `json:"search"`
	// Filter is "" (all), "uncategorized", "orphan" or "unused", depending on
	// the table.
	Filter string `json:"filter"`
}

// Page is one page of master rows.
type Page struct {
	Rows     []Row `json:"rows"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// Input is a row to create or update. OriginalKey is the key before an edit;
// empty means create. Editing a primary key is allowed for Item/Base/Category,
// but the store refuses to orphan referencing rows.
type Input struct {
	Table        string   `json:"table"`
	OriginalKey  string   `json:"originalKey"`
	ItemCode     string   `json:"itemCode"`
	BaseCode     string   `json:"baseCode"`
	ManuCode     string   `json:"manuCode"`
	SapCode      string   `json:"sapCode"`
	Category     string   `json:"category"`
	Material     string   `json:"material"`
	StandardCost *float64 `json:"standardCost"`
}

// Overview holds the counters and category choices shown above the table.
type Overview struct {
	ItemCount         int      `json:"itemCount"`
	BaseCount         int      `json:"baseCount"`
	SapCount          int      `json:"sapCount"`
	CategoryCount     int      `json:"categoryCount"`
	Uncategorized     int      `json:"uncategorized"`
	OrphanCount       int      `json:"orphanCount"`
	UnusedCategories  int      `json:"unusedCategories"`
	PriceCount        int      `json:"priceCount"`
	PriceMissing      int      `json:"priceMissing"`
	Categories        []string `json:"categories"`
	LastImportAt      string   `json:"lastImportAt"`
	LastPriceImportAt string   `json:"lastPriceImportAt"`
}

// ImportResult reports one workbook import.
type ImportResult struct {
	Path          string   `json:"path"`
	Name          string   `json:"name"`
	Items         int      `json:"items"`
	Bases         int      `json:"bases"`
	Categories    int      `json:"categories"`
	DuplicateRows int      `json:"duplicateRows"`
	Orphans       int      `json:"orphans"`
	Uncategorized int      `json:"uncategorized"`
	Warnings      []string `json:"warnings"`
	ImportedAt    string   `json:"importedAt"`
}

// PriceImportResult reports one price-workbook import.
type PriceImportResult struct {
	Path          string   `json:"path"`
	Name          string   `json:"name"`
	Prices        int      `json:"prices"`
	DuplicateRows int      `json:"duplicateRows"`
	MissingCosts  int      `json:"missingCosts"`
	SkippedRows   int      `json:"skippedRows"`
	Warnings      []string `json:"warnings"`
	ImportedAt    string   `json:"importedAt"`
}

// LookupResult is the read-only code-chain lookup.
type LookupResult struct {
	ItemCode string `json:"itemCode"`
	BaseCode string `json:"baseCode"`
	ManuCode string `json:"manuCode"`
	SapCode  string `json:"sapCode"`
	Category string `json:"category"`
	Rows     []Row  `json:"rows"`
}

// ReadPriceWorkbook opens path and reads Material and Standard_Cost. The header
// row is found in the first 20 rows, so the source report's first technical row
// and a normalized two-column workbook are both accepted. Only those two columns
// are read; every other column (description, hierarchy, plant, ...) is ignored.
func ReadPriceWorkbook(path string) (*PriceWorkbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开价格工作簿失败: %w", err)
	}
	defer f.Close()

	for _, sheet := range f.GetSheetList() {
		found, err := readPriceSheet(f, sheet)
		if err != nil {
			return nil, fmt.Errorf("读取价格工作表 %s 失败: %w", sheet, err)
		}
		if found != nil {
			found.Path = path
			found.Name = fileBase(path)
			return found, nil
		}
	}
	return nil, fmt.Errorf("找不到包含 Material / Standard_Cost 的工作表")
}

func readPriceSheet(f *excelize.File, sheet string) (*PriceWorkbook, error) {
	rows, err := f.Rows(sheet)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &PriceWorkbook{Prices: []PriceEntry{}}
	materialCol, costCol := -1, -1
	headerRow := 0
	rowNo := 0
	index := map[string]int{}
	for rows.Next() {
		rowNo++
		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}
		if materialCol < 0 {
			if rowNo > 20 {
				break
			}
			for c, value := range cols {
				switch headerKey(value) {
				case "material":
					materialCol = c
				case "standardcost":
					costCol = c
				}
			}
			if materialCol >= 0 && costCol >= 0 {
				headerRow = rowNo
			}
			continue
		}
		if rowNo <= headerRow {
			continue
		}
		material := cell(cols, materialCol)
		rawCost := cell(cols, costCol)
		if material == "" && rawCost == "" {
			continue
		}
		if material == "" {
			out.SkippedRows++
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("价格第 %d 行缺少 Material，已跳过", rowNo))
			continue
		}
		var cost *float64
		if rawCost != "" {
			numeric := strings.ReplaceAll(rawCost, ",", "")
			value, err := strconv.ParseFloat(numeric, 64)
			if err != nil {
				out.SkippedRows++
				out.Warnings = append(out.Warnings,
					fmt.Sprintf("Material %s 的 Standard_Cost %q 不是数字，已跳过（第 %d 行）", material, rawCost, rowNo))
				continue
			}
			cost = &value
		} else {
			out.MissingCosts++
		}
		entry := PriceEntry{Material: material, StandardCost: cost, SourceRow: rowNo}
		if old, ok := index[material]; ok {
			out.DuplicateRows++
			if !samePrice(out.Prices[old], entry) {
				out.Warnings = append(out.Warnings,
					fmt.Sprintf("Material %s 出现多次，保留最后一行（第 %d 行）", material, rowNo))
			}
			out.Prices[old] = entry
			continue
		}
		index[material] = len(out.Prices)
		out.Prices = append(out.Prices, entry)
	}
	if materialCol < 0 {
		return nil, nil
	}
	return out, rows.Error()
}

func samePrice(a, b PriceEntry) bool {
	if a.StandardCost == nil || b.StandardCost == nil {
		return a.StandardCost == nil && b.StandardCost == nil
	}
	return *a.StandardCost == *b.StandardCost
}

// ReadWorkbook opens path and reads the three mapping sheets. It is strict about
// the columns that carry the mapping (A–C) and ignores the analytical/formula
// columns to the right.
func ReadWorkbook(path string) (*Workbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开匹配工作簿失败: %w", err)
	}
	defer f.Close()

	itemRows, err := findSheetRows(f, "Item Code", "BASECODE")
	if err != nil {
		return nil, fmt.Errorf("Item → Base 表: %w", err)
	}
	baseRows, err := findSheetRows(f, "Base code", "Manu Planning Code", "SAP Code")
	if err != nil {
		return nil, fmt.Errorf("Base → SAP 表: %w", err)
	}
	categoryRows, err := findSheetRows(f, "Code", "Category")
	if err != nil {
		return nil, fmt.Errorf("SAP → Category 表: %w", err)
	}

	out := &Workbook{Path: path, Name: fileBase(path)}
	readItems(itemRows, out)
	readBases(baseRows, out)
	readCategories(categoryRows, out)
	return out, nil
}

func fileBase(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	if i >= 0 {
		return path[i+1:]
	}
	return path
}

type sheetData struct {
	header map[string]int
	rows   [][]string
}

// findSheetRows locates the sheet whose first 20 rows contain all required
// headers. Matching ignores case, spaces and punctuation, so "Item Code" and
// "ITEMCODE" are the same header.
func findSheetRows(f *excelize.File, required ...string) (*sheetData, error) {
	want := make([]string, len(required))
	for i, h := range required {
		want[i] = headerKey(h)
	}
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil || len(rows) == 0 {
			continue
		}
		limit := len(rows)
		if limit > 20 {
			limit = 20
		}
		for r := 0; r < limit; r++ {
			header := map[string]int{}
			for c, cell := range rows[r] {
				header[headerKey(cell)] = c
			}
			ok := true
			for _, key := range want {
				if _, exists := header[key]; !exists {
					ok = false
					break
				}
			}
			if ok {
				return &sheetData{header: header, rows: rows[r+1:]}, nil
			}
		}
	}
	return nil, fmt.Errorf("找不到包含 %s 的工作表", strings.Join(required, " / "))
}

func headerKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cell(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

func readItems(sd *sheetData, out *Workbook) {
	seen := map[string]ItemEntry{}
	conflict := map[string]bool{}
	for i, row := range sd.rows {
		item := cell(row, sd.header["itemcode"])
		base := cell(row, sd.header["basecode"])
		if item == "" && base == "" {
			continue
		}
		if item == "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Item → Base 第 %d 行缺少 Item Code，已跳过", i+2))
			continue
		}
		if base == "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Item → Base 第 %d 行（%s）缺少 Base Code，已跳过", i+2, item))
			continue
		}
		entry := ItemEntry{ItemCode: item, BaseCode: base, SourceRow: i + 2}
		old, ok := seen[item]
		if !ok {
			seen[item] = entry
			out.Items = append(out.Items, entry)
			continue
		}
		out.DuplicateRows++
		if old.BaseCode != base && !conflict[item] {
			conflict[item] = true
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("Item %s 对应多个 Base Code，保留第一行 %s（第 %d 行）", item, old.BaseCode, old.SourceRow))
		}
	}
}

func readBases(sd *sheetData, out *Workbook) {
	seen := map[string]BaseEntry{}
	conflict := map[string]bool{}
	for i, row := range sd.rows {
		base := cell(row, sd.header["basecode"])
		if base == "" {
			continue
		}
		if base == "*Item" {
			continue
		}
		entry := BaseEntry{
			BaseCode:  base,
			ManuCode:  cell(row, sd.header["manuplanningcode"]),
			SapCode:   cell(row, sd.header["sapcode"]),
			SourceRow: i + 2,
		}
		old, ok := seen[base]
		if !ok {
			seen[base] = entry
			out.Bases = append(out.Bases, entry)
			continue
		}
		out.DuplicateRows++
		if (old.ManuCode != entry.ManuCode || old.SapCode != entry.SapCode) && !conflict[base] {
			conflict[base] = true
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("Base Code %s 对应多组 SAP/Manu 值，保留第一行（第 %d 行）", base, old.SourceRow))
		}
	}
}

func readCategories(sd *sheetData, out *Workbook) {
	seen := map[string]CategoryEntry{}
	conflict := map[string]bool{}
	for i, row := range sd.rows {
		code := cell(row, sd.header["code"])
		if code == "" {
			continue
		}
		entry := CategoryEntry{SapCode: code, Category: cell(row, sd.header["category"]), SourceRow: i + 2}
		old, ok := seen[code]
		if !ok {
			seen[code] = entry
			out.Categories = append(out.Categories, entry)
			continue
		}
		out.DuplicateRows++
		if old.Category != entry.Category && !conflict[code] {
			conflict[code] = true
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("SAP Code %s 对应多个 Category，保留第一行 %q（第 %d 行）", code, old.Category, old.SourceRow))
		}
	}
}

// SortCategories returns a copy of the distinct non-empty category names, in
// case-insensitive order, for the editor's choices.
func SortCategories(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}
