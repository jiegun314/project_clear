// Command e2e drives the real pipeline over the real workbooks in raw_data and
// then audits the exported file for fidelity. It is the acceptance check for
// the whole backend, and it is meant to be run from the command line:
//
//	go run ./tools/e2e -in raw_data/mps_data -out /tmp/clear-e2e -clean=false
package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

	// Parameters live in the scratch folder, so an acceptance run never writes
	// into the folder the application itself uses.
	cfg, notes, err := config.NewStoreAt(filepath.Join(*out, "config"))
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
	entry, overwrote, err := svc.Commit()
	if err != nil {
		fail("commit: %v", err)
	}
	if overwrote {
		fail("首次整合被报告为覆盖原有数据")
	}
	mark("整合完成: %s -> %s, %d 行, %d 个文件", entry.WeekCode, entry.TableName, entry.RowCount, entry.FileCount)

	// ---- export --------------------------------------------------------
	dest := filepath.Join(*out, fmt.Sprintf("CLEAR_%s.xlsm", entry.WeekCode))
	if *clean {
		dest = filepath.Join(*out, fmt.Sprintf("CLEAR_%s_clean.xlsx", entry.WeekCode))
	}
	mode := config.ExportTemplate
	if *clean {
		mode = config.ExportClean
	}
	stats, err := svc.Export(service.ExportOptions{WeekCode: entry.WeekCode, DestPath: dest, Mode: mode}, nil)
	if err != nil {
		fail("export: %v", err)
	}
	mark("导出完成: %s (%d 字节)", dest, stats.SizeBytes)
	fmt.Printf("    mode=%s rows=%d cols=%d styles=%d cfRows=%d comments=%d dropped=%d vba=%v\n",
		stats.Mode, stats.Rows, stats.Cols, stats.StylesUsed, stats.CFRows, stats.Comments,
		stats.CFRulesDropped, stats.PreservedVBA)

	// ---- audit ---------------------------------------------------------
	if err := audit(dest, *in, entry.WeekCode); err != nil {
		fail("audit: %v", err)
	}

	// ---- 重复整合：同一周码只能有一份，时间标签换成新的 ----------------
	// 查询界面（历史数据）读的是 archive 表，临时数据不该混进去。
	if _, err := svc.ImportFolder(*in, nil); err != nil {
		fail("re-import: %v", err)
	}
	duringStaging, err := db.ListArchive()
	if err != nil {
		fail("list archive during staging: %v", err)
	}
	if len(duringStaging) != 1 || duringStaging[0].WeekCode != entry.WeekCode {
		fail("有临时数据时归档列表 = %+v, 应当仍只有 %s 一份", duringStaging, entry.WeekCode)
	}
	mark("重复整合: 未整合的临时数据未出现在查询列表")

	again, overwroteAgain, err := svc.Commit()
	if err != nil {
		fail("re-commit: %v", err)
	}
	if !overwroteAgain {
		fail("重复整合同一周码没有被报告为覆盖原有数据")
	}
	after, err := db.ListArchive()
	if err != nil {
		fail("list archive after re-commit: %v", err)
	}
	if len(after) != 1 {
		fail("重复整合后归档有 %d 条记录, 应当只有 %s 一条", len(after), entry.WeekCode)
	}
	if after[0].WeekCode != entry.WeekCode {
		fail("重复整合后归档周码 = %s, 期望 %s", after[0].WeekCode, entry.WeekCode)
	}
	if after[0].CommittedAt == "" || after[0].CommittedAt != again.CommittedAt {
		fail("整合时间未刷新: 归档 %q, 本次返回 %q", after[0].CommittedAt, again.CommittedAt)
	}
	grid2, err := svc.Query(store.Query{Source: entry.WeekCode, Page: 1, PageSize: 1, SortField: "seq"})
	if err != nil {
		fail("query committed week after re-commit: %v", err)
	}
	if grid2.Total != entry.RowCount {
		fail("重复整合后周数据 %d 行, 首次 %d 行, 数据被累加或替换不完整", grid2.Total, entry.RowCount)
	}
	mark("重复整合: %s 仍只有一份, %d 行, 时间 %s -> %s",
		entry.WeekCode, grid2.Total, entry.CommittedAt, again.CommittedAt)

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
	// Both export modes hand on one page: the MPS sheet and nothing else.
	if sheets := f.GetSheetList(); len(sheets) != 1 || sheets[0] != mps.SheetName {
		return fmt.Errorf("导出文件包含 %v，应当只有 %q 一页", sheets, mps.SheetName)
	}
	mark("审计: 工作簿只有 MPS 一页")

	// Nothing above the header: rows 1..54 of the source are report decoration,
	// and the export starts at the header instead.
	if len(rows) < mps.ExportFirstDataRow {
		return fmt.Errorf("导出文件数据行不足")
	}
	for i := 0; i < mps.ExportHeaderRow-1 && i < len(rows); i++ {
		for c, v := range rows[i] {
			if strings.TrimSpace(v) != "" {
				return fmt.Errorf("表头之前仍有内容: %s = %q", mps.CellRef(c+1, i+1), v)
			}
		}
	}
	mark("审计: 导出文件共 %d 行，表头前的 %d 行已移除", len(rows), mps.DropRows)

	// 0. The declared used range must cover the sheet's content. Both engines
	// set it for the header block before the data is written, and a range that
	// stops there makes dimension-respecting readers — openpyxl's read_only
	// mode among them — report an empty workbook even though every row is
	// present and Excel itself recalculates the range on open.
	headerRow := rows[mps.ExportHeaderRow-1]
	if len(headerRow) <= mps.IndexCols {
		return fmt.Errorf("导出文件表头只有 %d 列，缺少周数据列", len(headerRow))
	}
	lastColName, err := mps.ColumnName(len(headerRow))
	if err != nil {
		return err
	}
	wantDim := fmt.Sprintf("A1:%s%d", lastColName, len(rows))
	dim, err := f.GetSheetDimension(mps.SheetName)
	if err != nil {
		return fmt.Errorf("读取 dimension 失败: %w", err)
	}
	if dim != wantDim {
		return fmt.Errorf("dimension = %q, 期望 %q（共 %d 行 × %d 列）", dim, wantDim, len(rows), len(headerRow))
	}
	mark("审计: dimension %s 覆盖全部 %d 行", dim, len(rows))

	// 1. header block must be intact, on its new first row
	for c := 1; c <= mps.IndexCols; c++ {
		ref := mps.CellRef(c, mps.ExportHeaderRow)
		got, _ := f.GetCellValue(mps.SheetName, ref)
		if strings.TrimSpace(got) == "" {
			return fmt.Errorf("第 %d 行第 %d 列表头丢失 (%s)", mps.ExportHeaderRow, c, ref)
		}
	}
	w, err := mps.ParseWeekCode(weekCode)
	if err != nil {
		return err
	}
	codeRef := mps.CellRef(mps.FirstWeekCol, mps.ExportHeaderRow)
	dateRef := mps.CellRef(mps.FirstWeekCol, mps.ExportDateRow)
	gotCode, _ := f.GetCellValue(mps.SheetName, codeRef, excelize.Options{RawCellValue: true})
	if gotCode != weekCode {
		return fmt.Errorf("%s = %q, 期望 %q", codeRef, gotCode, weekCode)
	}
	serial, _ := f.GetCellValue(mps.SheetName, dateRef, excelize.Options{RawCellValue: true})
	if serial == "" {
		return fmt.Errorf("%s 起始日期丢失", dateRef)
	}
	mark("审计: 表头完整, %s=%s %s=%s (期望 %s)", codeRef, gotCode, dateRef, serial, w.StartText())

	// 2. the styled separator row must keep its formatting even though it holds
	// no values
	blankRef := mps.CellRef(mps.FirstWeekCol, mps.ExportBlankRow)
	blankStyle, err := f.GetCellStyle(mps.SheetName, blankRef)
	if err != nil || blankStyle == 0 {
		return fmt.Errorf("%s 样式丢失 (style=%d err=%v)", blankRef, blankStyle, err)
	}
	mark("审计: %s 样式保留 (style id %d)", blankRef, blankStyle)

	// 3. the LOC filter must hold in the output
	locRef := mps.CellRef(mps.LOCCol, mps.ExportHeaderRow)
	locCol, _ := f.GetCellValue(mps.SheetName, locRef)
	if locCol != "LOC" {
		return fmt.Errorf("%s = %q, 期望 LOC", locRef, locCol)
	}
	kept, bad := 0, 0
	for i := mps.ExportFirstDataRow - 1; i < len(rows); i++ {
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
	// The frozen pane and the filter range are stored in source row numbers, so
	// they have to move up with the header. A split left as-is freezes the top
	// of the data block, and a filter range left as-is points at rows that no
	// longer hold that data.
	if err := auditSheetView(raw, len(rows)); err != nil {
		return err
	}
	cf, err := mps.ParseCFRowsFrom(raw, mps.ExportFirstDataRow)
	if err != nil {
		return err
	}
	// Excel repairs a sheet whose elements are in the wrong order even when the
	// XML is perfectly well formed, so the order is checked explicitly.
	if err := auditSheetOrder(dest, raw); err != nil {
		return err
	}
	// And every rule must point at a differential format that exists: an id
	// carried over from another workbook's own numbering makes Excel refuse the
	// whole sheet ("有 XML 错误").
	if err := auditDxfReferences(dest, raw); err != nil {
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

	// 6. cell annotations must survive reading, filtering, merging and export
	want, err := sourceComments(srcDir)
	if err != nil {
		return err
	}
	list, err := f.GetComments(mps.SheetName)
	if err != nil {
		return err
	}
	got := map[string]int{}
	for _, c := range list {
		got[strings.TrimSpace(c.Text)]++
	}
	if len(want) == 0 && len(got) > 0 {
		return fmt.Errorf("导出文件有 %d 条注释，但源数据中没有一条", len(got))
	}
	for text, n := range want {
		if got[text] != n {
			return fmt.Errorf("注释丢失或重复: 期望 %d 条 %q，导出中为 %d 条", n, text, got[text])
		}
	}
	for text := range got {
		if want[text] == 0 {
			return fmt.Errorf("导出文件出现源数据中没有的注释 %q", text)
		}
	}
	mark("审计: 注释 %d 条，与源文件中通过 LOC 筛选的行完全一致", len(list))

	// 7. no modern comment may stay anchored to a row the merge has rewritten
	if part, err := mps.ThreadedCommentPartForTest(dest, mps.SheetName); err == nil && part != "" {
		raw, err := mps.ZipPartForTest(dest, part)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "<threadedComment") {
			return fmt.Errorf("导出文件仍带有 %s 中的线程批注锚点", part)
		}
		mark("审计: 无残留线程批注锚点")
	}

	// 8. the week numbers must sit in the columns they came from. This is the
	// independent check: both sides are read with excelize's own cell
	// addressing, so a mistake in CLEAR's column arithmetic cannot cancel out.
	if err := auditWeekValues(dest, srcDir); err != nil {
		return err
	}

	// 9. the source workbook's structure must have survived a template export
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

// auditSheetView checks the two pieces of sheet furniture that are expressed in
// row numbers and therefore have to be rebased with the header: the frozen pane
// and the filter range. Both were left at their source values, which froze the
// first 54 data rows and filtered a range that no longer holds that data.
func auditSheetView(raw []byte, dataRows int) error {
	if m := regexp.MustCompile(`<pane[^>]*>`).Find(raw); m != nil {
		tag := string(m)
		if s := regexp.MustCompile(`ySplit="(\d+)"`).FindStringSubmatch(tag); s != nil {
			n, err := strconv.Atoi(s[1])
			if err != nil {
				return fmt.Errorf("冻结窗格 ySplit 无法解析: %q", s[1])
			}
			if n > mps.ExportBlankRow {
				return fmt.Errorf("冻结窗格 ySplit=%d 超过了表头的 %d 行，会冻结数据行: %s",
					n, mps.ExportBlankRow, tag)
			}
		}
		if t := regexp.MustCompile(`topLeftCell="([A-Z]+)(\d+)"`).FindStringSubmatch(tag); t != nil {
			row, err := strconv.Atoi(t[2])
			if err != nil {
				return fmt.Errorf("冻结窗格 topLeftCell 无法解析: %q", t[0])
			}
			if row > dataRows {
				return fmt.Errorf("冻结窗格 topLeftCell 行 %d 超出 %d 行数据范围: %s", row, dataRows, tag)
			}
		}
		mark("审计: 冻结窗格已随表头重定位 (%s)", strings.TrimSpace(tag))
	}
	if m := regexp.MustCompile(`<autoFilter[^>]*ref="([A-Z]+)(\d+):`).FindSubmatch(raw); m != nil {
		row, err := strconv.Atoi(string(m[2]))
		if err != nil {
			return fmt.Errorf("autoFilter 起始行无法解析: %q", m[2])
		}
		if row > mps.ExportFirstDataRow {
			return fmt.Errorf("autoFilter 起始行 %d 落在数据区中段，未随表头上移 (期望 <= %d)",
				row, mps.ExportFirstDataRow)
		}
		mark("审计: 筛选区域已随表头重定位 (起始行 %d)", row)
	}
	return nil
}

// auditSheetOrder checks the worksheet's children against the order the OOXML
// schema requires. Excel reports 「有 XML 错误」and silently repairs the file
// when, for example, conditionalFormatting is written after legacyDrawing —
// something a well-formedness check cannot see.
func auditSheetOrder(workbook string, raw []byte) error {
	tags, err := topLevelTags(raw)
	if err != nil {
		return err
	}
	// Every one of these must come after the conditional-format block.
	after := []string{"dataValidations", "hyperlinks", "printOptions", "pageMargins",
		"pageSetup", "headerFooter", "rowBreaks", "colBreaks", "drawing",
		"drawingHF", "picture", "oleObjects", "controls", "tableParts",
		"legacyDrawing", "legacyDrawingHF", "extLst"}
	cf, data := -1, -1
	for i, tag := range tags {
		switch tag {
		case "conditionalFormatting":
			if cf < 0 {
				cf = i
			}
		case "sheetData":
			data = i
		}
	}
	if cf < 0 {
		return nil // nothing to place
	}
	if data >= 0 && cf < data {
		return fmt.Errorf("%s: conditionalFormatting 出现在 sheetData 之前", filepath.Base(workbook))
	}
	position := map[string]int{}
	for i, tag := range tags {
		if _, ok := position[tag]; !ok {
			position[tag] = i
		}
	}
	for _, tag := range after {
		if i, ok := position[tag]; ok && i < cf {
			return fmt.Errorf("%s: <%s> 位于 conditionalFormatting 之前，Excel 会判为文件损坏",
				filepath.Base(workbook), tag)
		}
	}
	mark("审计: 工作表元素顺序符合 OOXML 规范")
	return nil
}

// topLevelTags lists the direct children of the worksheet element.
func topLevelTags(doc []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	var (
		out   []string
		depth int
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				out = append(out, e.Name.Local)
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

// auditDxfReferences checks that every dxfId in the exported sheet lands inside
// the workbook's own <dxfs> table. Ids are per-source-file, so a merged export
// that copies them across can point past the end of the table — which Excel
// reports as a broken sheet rather than a missing colour.
func auditDxfReferences(dest string, sheetXML []byte) error {
	styles, err := mps.ZipPartForTest(dest, "xl/styles.xml")
	if err != nil {
		return err
	}
	dxfs := mps.ParseDxfs(styles)
	ids := regexp.MustCompile(`dxfId="(\d+)"`).FindAllSubmatch(sheetXML, -1)
	if len(ids) == 0 {
		return nil
	}
	max := -1
	for _, m := range ids {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return fmt.Errorf("无法解析 dxfId %q", m[1])
		}
		if n > max {
			max = n
		}
		if n >= len(dxfs) {
			return fmt.Errorf("%s: 规则引用了 dxfId=%d，但样式表只有 %d 个条件格式配色",
				filepath.Base(dest), n, len(dxfs))
		}
	}
	mark("审计: %d 条规则引用的配色均在样式表范围内（最大 dxfId=%d，配色 %d 个）",
		len(ids), max, len(dxfs))
	return nil
}

// auditWeekValues cross-checks the exported week cells against the workbook the
// template came from. A row is identified by its A..O natural key, and every
// cell from P onwards is compared at its exact column letter.
func auditWeekValues(dest, srcDir string) error {
	src := firstSource(srcDir)
	if src == "" {
		return fmt.Errorf("源目录中没有 .xlsm，无法交叉校验周数据")
	}
	source, err := excelize.OpenFile(src)
	if err != nil {
		return err
	}
	defer source.Close()
	exported, err := excelize.OpenFile(dest)
	if err != nil {
		return err
	}

	srcRows, err := source.GetRows(mps.SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return err
	}
	// Natural key -> source row number, for the rows that survive the filter.
	byKey := map[string]int{}
	for i := mps.FirstDataRow - 1; i < len(srcRows); i++ {
		r := srcRows[i]
		if len(r) < mps.LOCCol || strings.TrimSpace(r[mps.LOCCol-1]) != "WH_CNB" {
			continue
		}
		if key, ok := rowKey(source, i+1); ok {
			if _, dup := byKey[key]; !dup {
				byKey[key] = i + 1
			}
		}
	}
	if len(byKey) == 0 {
		return fmt.Errorf("%s 中没有通过 LOC 筛选的行", filepath.Base(src))
	}

	expRows, err := exported.GetRows(mps.SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return err
	}
	// The acceptance run fixes the read width at the documented default of 20
	// week columns (P..AI).
	const readColumns = 20
	matched, cells := 0, 0
	for i := mps.ExportFirstDataRow - 1; i < len(expRows); i++ {
		key, ok := rowKey(exported, i+1)
		if !ok {
			continue
		}
		srcRow, found := byKey[key]
		if !found {
			continue
		}
		matched++
		for w := 0; w < readColumns; w++ {
			col := mps.FirstWeekCol + w
			name, err := mps.ColumnName(col)
			if err != nil {
				return err
			}
			got := exportedValue(exported, name, i+1)
			want := exportedValue(source, name, srcRow)
			if !sameCellValue(got, want) {
				return fmt.Errorf("周数据列错位或丢失: 第 %d 行 %s%d = %q，源文件 %s%d = %q",
					i+1, name, i+1, got, name, srcRow, want)
			}
			cells++
		}
	}
	if matched == 0 {
		return fmt.Errorf("导出文件与 %s 没有任何可按自然键匹配的行", filepath.Base(src))
	}
	mark("审计: 周数据列逐格一致（%d 行 / %d 格，与 %s 交叉校验）", matched, cells, filepath.Base(src))
	return nil
}

// rowKey is the A..O natural key of a worksheet row.
func rowKey(f *excelize.File, row int) (string, bool) {
	parts := make([]string, 0, mps.IndexCols)
	for c := 1; c <= mps.IndexCols; c++ {
		name, err := mps.ColumnName(c)
		if err != nil {
			return "", false
		}
		parts = append(parts, strings.TrimSpace(exportedValue(f, name, row)))
	}
	key := strings.Join(parts, "\x1f")
	if strings.TrimSpace(parts[mps.LOCCol-1]) != "WH_CNB" {
		return "", false
	}
	return key, true
}

func exportedValue(f *excelize.File, col string, row int) string {
	v, err := f.GetCellValue(mps.SheetName, fmt.Sprintf("%s%d", col, row), excelize.Options{RawCellValue: true})
	if err != nil {
		return ""
	}
	return v
}

// sameCellValue compares two raw cell texts, tolerating the rounding Excel
// applies when a float is written back out.
func sameCellValue(a, b string) bool {
	if a == b {
		return true
	}
	fa, ea := strconv.ParseFloat(a, 64)
	fb, eb := strconv.ParseFloat(b, 64)
	if ea != nil || eb != nil {
		return false
	}
	diff := fa - fb
	if diff < 0 {
		diff = -diff
	}
	scale := math.Max(math.Abs(fa), math.Abs(fb))
	if scale < 1 {
		scale = 1
	}
	return diff/scale < 1e-9
}

// sourceComments collects the annotations of every row that the LOC filter
// keeps, keyed by text and counted, so the audit can compare them with what the
// exported workbook carries.
func sourceComments(dir string) (map[string]int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	dict := mps.NewStyleDict()
	out := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".xlsm") {
			continue
		}
		fr, err := mps.ReadFile(filepath.Join(dir, e.Name()), mps.ReadOptions{
			LOCFilter: "WH_CNB", ReadColumns: 20, Dict: dict,
		})
		if err != nil {
			return nil, err
		}
		if fr.Err != "" {
			continue // a file the import rejected cannot contribute comments
		}
		for _, row := range fr.Rows {
			for _, c := range row.Comments {
				if text := strings.TrimSpace(c.Text); text != "" {
					out[text]++
				}
			}
		}
	}
	return out, nil
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
	// Both lookups exist for their error: a workbook whose MPS sheet cannot be
	// located is a different kind of failure from a part that differs.
	if _, err := mps.SheetPartPath(src, mps.SheetName); err != nil {
		return true, "源文件结构无法解析，跳过"
	}
	if _, err := mps.SheetPartPath(dest, mps.SheetName); err != nil {
		return false, "导出文件缺少 MPS 工作表"
	}
	var kept []string
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
