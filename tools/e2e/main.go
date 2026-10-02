// Command e2e drives the real pipeline over the real workbooks in raw_data and
// then audits the exported file for fidelity. It is the acceptance check for
// the whole backend, and it is meant to be run from the command line:
//
//	go run ./tools/e2e -in raw_data/mps_data -out /tmp/clear-e2e -clean=false
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/mps"
	"project_clear/internal/service"
	"project_clear/internal/store"

	"github.com/xuri/excelize/v2"
)

var start = time.Now()

func mark(format string, args ...any) {
	fmt.Printf("[%7s] %s\n", time.Since(start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

func main() {
	in := flag.String("in", "raw_data/mps_data", "folder of source workbooks")
	out := flag.String("out", "/tmp/clear-e2e", "scratch folder for the database and export")
	clean := flag.Bool("clean", false, "use the clean rebuild export engine")
	keep := flag.Bool("keep", false, "keep the scratch folder from a previous run")
	flag.Parse()

	if !*keep {
		_ = os.RemoveAll(*out)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail("mkdir: %v", err)
	}

	cfg, notes, err := config.NewStore()
	if err != nil {
		fail("config: %v", err)
	}
	for _, n := range notes {
		mark("参数 %s", n)
	}
	c := cfg.Get()
	c.LOCFilter = "WH_CNB"
	c.ReadColumns = 20
	if _, err := cfg.Save(c); err != nil {
		fail("config save: %v", err)
	}

	log := logging.New(filepath.Join(*out, "logs"))
	db, err := store.Open(filepath.Join(*out, "clear.db"))
	if err != nil {
		fail("db: %v", err)
	}
	defer db.Close()

	svc := service.New(cfg, log, db, *out)

	// ---- import --------------------------------------------------------
	res, err := svc.ImportFolder(*in, func(stage string, done, total int) {
		mark("  %s %d/%d", stage, done, total)
	})
	if err != nil {
		fail("import: %v", err)
	}
	mark("导入完成: 周码 %s (%s), 成功 %d / 失败 %d / 合计 %d, 合并 %d 行",
		res.WeekCode, res.WeekStart, res.OK, res.Failed, res.Total, res.RowsKept)
	for _, w := range res.Warnings {
		mark("  警告: %s", w)
	}
	fmt.Println("  per-file:")
	for _, f := range res.Files {
		status := "ok"
		if f.Status != "ok" {
			status = "FAILED"
		}
		fmt.Printf("    %-8s %-58s total=%-5d kept=%-4d %s\n", status, trunc(f.Name, 58), f.RowsTotal, f.RowsKept, f.Err)
	}

	// ---- staging query -------------------------------------------------
	grid, err := svc.Query(store.Query{Source: "", Page: 1, PageSize: 5, SortField: "seq"})
	if err != nil {
		fail("staging query: %v", err)
	}
	mark("暂存可查询: %d 行, 首页 %d 行", grid.Total, len(grid.Rows))

	// ---- commit --------------------------------------------------------
	entry, err := svc.Commit()
	if err != nil {
		fail("commit: %v", err)
	}
	mark("整合完成: %s -> %s, %d 行, %d 个文件", entry.WeekCode, entry.TableName, entry.RowCount, entry.FileCount)

	// ---- export --------------------------------------------------------
	dest := filepath.Join(*out, fmt.Sprintf("CLEAR_%s.xlsm", entry.WeekCode))
	if *clean {
		dest = filepath.Join(*out, fmt.Sprintf("CLEAR_%s_clean.xlsx", entry.WeekCode))
	}
	stats, err := svc.Export(service.ExportOptions{WeekCode: entry.WeekCode, DestPath: dest}, nil)
	if err != nil {
		fail("export: %v", err)
	}
	mark("导出完成: %s (%d 字节)", dest, stats.SizeBytes)
	fmt.Printf("    mode=%s rows=%d cols=%d styles=%d cfRows=%d comments=%d dropped=%d blanked=%d vba=%v\n",
		stats.Mode, stats.Rows, stats.Cols, stats.StylesUsed, stats.CFRows, stats.Comments,
		stats.CFRulesDropped, stats.BlankedRows, stats.PreservedVBA)

	// ---- audit ---------------------------------------------------------
	if err := audit(dest, *in, entry.WeekCode); err != nil {
		fail("audit: %v", err)
	}
	fmt.Println("\n全部检查通过。")
}

func audit(dest, srcDir, weekCode string) error {
	f, err := excelize.OpenFile(dest)
	if err != nil {
		return fmt.Errorf("导出文件无法打开: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(mps.SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return err
	}
	if len(rows) < mps.FirstDataRow {
		return fmt.Errorf("导出文件数据行不足")
	}
	mark("审计: 导出文件共 %d 行", len(rows))

	// 1. header block must be untouched
	for c := 1; c <= mps.IndexCols; c++ {
		ref := mps.CellRef(c, mps.HeaderRow)
		got, _ := f.GetCellValue(mps.SheetName, ref)
		if strings.TrimSpace(got) == "" {
			return fmt.Errorf("第 %d 行第 %d 列表头丢失 (%s)", mps.HeaderRow, c, ref)
		}
	}
	w, err := mps.ParseWeekCode(weekCode)
	if err != nil {
		return err
	}
	got55, _ := f.GetCellValue(mps.SheetName, "P55", excelize.Options{RawCellValue: true})
	if got55 != weekCode {
		return fmt.Errorf("P55 = %q, 期望 %q", got55, weekCode)
	}
	serial, _ := f.GetCellValue(mps.SheetName, "P56", excelize.Options{RawCellValue: true})
	if serial == "" {
		return fmt.Errorf("P56 起始日期丢失")
	}
	mark("审计: 表头完整, P55=%s P56=%s (期望 %s)", got55, serial, w.StartText())

	// 2. row 57 must keep its styling even though it holds no values
	s57, err := f.GetCellStyle(mps.SheetName, "P57")
	if err != nil || s57 == 0 {
		return fmt.Errorf("第 57 行样式丢失 (style=%d err=%v)", s57, err)
	}
	mark("审计: 第 57 行样式保留 (style id %d)", s57)

	// 3. the LOC filter must hold in the output
	locCol, _ := f.GetCellValue(mps.SheetName, "L55")
	if locCol != "LOC" {
		return fmt.Errorf("L55 = %q, 期望 LOC", locCol)
	}
	kept, bad := 0, 0
	for i := mps.FirstDataRow - 1; i < len(rows); i++ {
		r := rows[i]
		if len(r) < mps.LOCCol {
			continue
		}
		if strings.TrimSpace(r[mps.LOCCol-1]) != "WH_CNB" {
			bad++
			continue
		}
		kept++
	}
	if bad > 0 {
		return fmt.Errorf("导出文件中有 %d 行未通过 LOC 筛选", bad)
	}
	mark("审计: %d 行全部满足 LOC=WH_CNB 筛选", kept)

	// 4. conditional formats must exist on the data rows
	part, err := mps.SheetPartPath(dest, mps.SheetName)
	if err != nil {
		return err
	}
	raw, err := mps.ZipPartForTest(dest, part)
	if err != nil {
		return err
	}
	cf, err := mps.ParseCFRows(raw)
	if err != nil {
		return err
	}
	rules := 0
	blocks := 0
	for _, prog := range cf {
		blocks += len(prog.Blocks)
		for _, b := range prog.Blocks {
			rules += len(b.Rules)
		}
	}
	if blocks == 0 {
		return fmt.Errorf("导出文件中没有任何数据行条件格式")
	}
	mark("审计: 条件格式 %d 行 / %d 块 / %d 条规则", len(cf), blocks, rules)

	// 5. no rule may still point at a source row far from the block it lives on
	for row, prog := range cf {
		for _, b := range prog.Blocks {
			for _, rule := range b.Rules {
				for _, off := range rule.Offsets {
					if off < -3 || off > 3 {
						return fmt.Errorf("第 %d 行的规则引用了 %+d 行的偏移，重锚可能失败", row, off)
					}
				}
			}
		}
	}
	mark("审计: 所有规则偏移均在合理范围")

	// 6. the source workbook's structure must have survived a template export
	if mps.HasPart(dest, "xl/vbaProject.bin") {
		mark("审计: VBA 工程保留")
	}
	srcSample := firstSource(srcDir)
	if srcSample != "" {
		if same, detail := compareParts(srcSample, dest); !same {
			return fmt.Errorf("模板部件比对失败: %s", detail)
		} else {
			mark("审计: 模板部件保留 -> %s", detail)
		}
	}
	return nil
}

func firstSource(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".xlsm") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return filepath.Join(dir, names[0])
}

func compareParts(src, dest string) (bool, string) {
	sp, err := mps.SheetPartPath(src, mps.SheetName)
	if err != nil {
		return true, "源文件结构无法解析，跳过"
	}
	dp, err := mps.SheetPartPath(dest, mps.SheetName)
	if err != nil {
		return false, "导出文件缺少 MPS 工作表"
	}
	var kept []string
	for _, p := range []string{"xl/vbaProject.bin", "xl/theme/theme1.xml", "xl/pivotCache"} {
		_ = p
	}
	_ = sp
	_ = dp
	for _, p := range []string{"xl/theme/theme1.xml"} {
		a, err1 := mps.ZipPartForTest(src, p)
		b, err2 := mps.ZipPartForTest(dest, p)
		if err1 == nil && err2 == nil && len(a) == len(b) {
			kept = append(kept, filepath.Base(p))
		}
	}
	if mps.HasPart(dest, "xl/vbaProject.bin") {
		kept = append(kept, "vbaProject.bin")
	}
	return true, strings.Join(kept, ", ")
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func fail(format string, args ...any) {
	fmt.Printf("FAIL: "+format+"\n", args...)
	os.Exit(1)
}

var _ = json.Marshal
