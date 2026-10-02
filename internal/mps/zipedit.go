package mps

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	dxfsOpen  = "<dxfs"
	dxfsClose = "</dxfs>"
)

// excelize can read and write values, styles and comments faithfully, but its
// conditional-format API loses rules, so the final sheet XML is patched after
// the save. Rewriting a single zip entry in place is the least invasive way to
// do that: every other part is copied over byte for byte, which is what keeps
// vbaProject.bin, slicers, pivot caches and ActiveX controls intact.

// zipPart reads a single entry out of a workbook.
func zipPart(path, name string) ([]byte, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("压缩包中找不到 %s", name)
}

// SheetPartPath resolves the zip path of a worksheet by its display name.
func SheetPartPath(workbook, sheet string) (string, error) {
	wb, err := zipPart(workbook, "xl/workbook.xml")
	if err != nil {
		return "", err
	}
	var book struct {
		Sheets struct {
			Sheet []struct {
				Name string `xml:"name,attr"`
				RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
			} `xml:"sheet"`
		} `xml:"sheets"`
	}
	if err := xml.Unmarshal(wb, &book); err != nil {
		return "", err
	}
	var rid string
	for _, s := range book.Sheets.Sheet {
		if s.Name == sheet {
			rid = s.RID
			break
		}
	}
	if rid == "" {
		return "", fmt.Errorf("工作簿中找不到工作表 %q", sheet)
	}
	rels, err := zipPart(workbook, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rel struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(rels, &rel); err != nil {
		return "", err
	}
	for _, r := range rel.Rel {
		if r.ID != rid {
			continue
		}
		target := r.Target
		if strings.HasPrefix(target, "/") {
			return strings.TrimPrefix(target, "/"), nil
		}
		target = strings.TrimPrefix(target, "xl/")
		return "xl/" + target, nil
	}
	return "", fmt.Errorf("找不到工作表 %q 的关系条目", sheet)
}

// HasPart reports whether the workbook contains a zip entry.
func HasPart(workbook, name string) bool {
	_, err := zipPart(workbook, name)
	return err == nil
}

// ReplaceParts rewrites the given zip entries of a workbook in place. Entries
// that are not listed are copied with their original compressed bytes.
func ReplaceParts(workbook string, replacements map[string][]byte) error {
	zr, err := zip.OpenReader(workbook)
	if err != nil {
		return err
	}
	defer zr.Close()

	dir := filepath.Dir(workbook)
	tmp, err := os.CreateTemp(dir, ".clear-export-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	zw := zip.NewWriter(tmp)
	seen := map[string]bool{}
	for _, f := range zr.File {
		newData, replace := replacements[f.Name]
		if !replace {
			// CopyRaw keeps untouched parts bit-identical.
			rc, err := f.OpenRaw()
			if err != nil {
				tmp.Close()
				zw.Close()
				return err
			}
			hdr := f.FileHeader
			w, err := zw.CreateRaw(&hdr)
			if err != nil {
				tmp.Close()
				zw.Close()
				return err
			}
			if _, err := io.Copy(w, rc); err != nil {
				tmp.Close()
				zw.Close()
				return err
			}
			seen[f.Name] = true
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		if err != nil {
			tmp.Close()
			zw.Close()
			return err
		}
		if _, err := w.Write(newData); err != nil {
			tmp.Close()
			zw.Close()
			return err
		}
		seen[f.Name] = true
	}
	// Parts that did not exist in the original (rare, but possible when a
	// clean export gains a part) are appended.
	for name, data := range replacements {
		if seen[name] {
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			tmp.Close()
			zw.Close()
			return err
		}
		if _, err := w.Write(data); err != nil {
			tmp.Close()
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, workbook)
}

// InjectConditionalFormats rewrites a saved workbook so that the data rows
// carry the supplied conditional-format programs. Header-area formats are kept
// untouched, and priorities are renumbered above whatever survives.
func InjectConditionalFormats(workbook, sheet string, rows map[int]CFRow, lastCol int) error {
	part, err := SheetPartPath(workbook, sheet)
	if err != nil {
		return err
	}
	raw, err := zipPart(workbook, part)
	if err != nil {
		return err
	}
	stripped := StripDataAreaCF(raw)
	if len(rows) == 0 {
		return ReplaceParts(workbook, map[string][]byte{part: stripped})
	}

	prepared := make(map[int]CFRow, len(rows))
	for row, prog := range rows {
		prepared[row] = clipProgram(prog, lastCol)
	}

	xmlStr, _, err := RenderCFRows(prepared, MaxCFPriority(stripped)+1)
	if err != nil {
		return err
	}
	merged, err := insertBeforeSheetEnd(string(stripped), xmlStr)
	if err != nil {
		return err
	}
	return ReplaceParts(workbook, map[string][]byte{part: []byte(merged)})
}

// clipProgram limits a row program to the exported column span and drops rules
// whose formulas would reach outside the exported table.
func clipProgram(in CFRow, lastCol int) CFRow {
	out := CFRow{}
	for _, blk := range in.Blocks {
		c1, c2 := blk.C1, blk.C2
		if c1 < FirstWeekCol {
			c1 = FirstWeekCol
		}
		if c2 > lastCol {
			c2 = lastCol
		}
		if c2 < c1 {
			continue
		}
		out.Blocks = append(out.Blocks, CFBlock{C1: c1, C2: c2, Rules: blk.Rules})
	}
	return out
}

// insertBeforeSheetEnd places the generated XML at the position the schema
// requires: conditionalFormatting comes after mergeCells and before
// dataValidations, hyperlinks, printOptions, pageMargins and pageSetup.
func insertBeforeSheetEnd(doc, fragment string) (string, error) {
	if fragment == "" {
		return doc, nil
	}
	anchors := []string{
		"<dataValidations", "<hyperlinks", "<printOptions",
		"<pageMargins", "<pageSetup", "<headerFooter", "<rowBreaks", "</worksheet>",
	}
	idx := -1
	for _, a := range anchors {
		if i := strings.Index(doc, a); i >= 0 && (idx < 0 || i < idx) {
			idx = i
		}
	}
	if idx < 0 {
		return "", fmt.Errorf("无法定位 </worksheet>，条件格式写入失败")
	}
	return doc[:idx] + fragment + doc[idx:], nil
}

// CopyDxfsInto grafts the source workbook's <dxfs> block into a clean
// workbook's styles.xml and returns the offset applied to dxfIds.
//
// A clean export starts with no differential formats, so the template's dxfs
// can be adopted wholesale and the original dxfIds stay valid. When the target
// already has dxfs they are kept and the source ones are appended.
func CopyDxfsInto(targetStyles, sourceStyles []byte) ([]byte, int, bool, error) {
	srcBlock, srcCount, ok := extractDxfs(sourceStyles)
	if !ok || srcCount == 0 {
		return targetStyles, 0, false, nil
	}
	existing, existingCount, hasExisting := extractDxfs(targetStyles)
	if !hasExisting {
		// No dxfs element at all: insert one after cellXfs, which the schema
		// requires to precede it.
		idx := strings.Index(string(targetStyles), "</cellXfs>")
		if idx < 0 {
			return nil, 0, false, fmt.Errorf("目标样式表缺少 </cellXfs>，无法写入条件格式")
		}
		at := idx + len("</cellXfs>")
		doc := string(targetStyles)
		return []byte(doc[:at] + srcBlock + doc[at:]), 0, true, nil
	}
	if existingCount == 0 {
		// An empty <dxfs count="0"/> placeholder can be filled in place.
		newBlock := fmt.Sprintf(`<dxfs count="%d">%s</dxfs>`, srcCount, innerOf(srcBlock))
		return []byte(strings.Replace(string(targetStyles), existing, newBlock, 1)), 0, true, nil
	}
	// Append: rewrite both count and body.
	merged := fmt.Sprintf(`<dxfs count="%d">%s%s</dxfs>`,
		existingCount+srcCount, innerOf(existing), innerOf(srcBlock))
	return []byte(strings.Replace(string(targetStyles), existing, merged, 1)), existingCount, true, nil
}

func extractDxfs(styles []byte) (block string, count int, ok bool) {
	doc := string(styles)
	i := strings.Index(doc, dxfsOpen)
	if i < 0 {
		return "", 0, false
	}
	// Distinguish <dxfs ...> from <dxfs count="0"/>
	if j := strings.Index(doc[i:], ">"); j >= 0 {
		if seg := doc[i : i+j]; strings.Contains(seg, `count="0"`) {
			return doc[i : i+j+1], 0, true
		}
	}
	closeIdx := strings.Index(doc[i:], dxfsClose)
	if closeIdx < 0 {
		return "", 0, false
	}
	block = doc[i : i+closeIdx+len(dxfsClose)]
	fmt.Sscanf(attrOrEmpty(block, "count"), "%d", &count)
	return block, count, true
}

func attrOrEmpty(block, name string) string {
	headEnd := strings.Index(block, ">")
	if headEnd < 0 {
		return "0"
	}
	v, err := attrValue(block[1:headEnd], name)
	if err != nil {
		return "0"
	}
	return v
}

func innerOf(block string) string {
	start := strings.Index(block, ">")
	end := strings.LastIndex(block, "</dxfs>")
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return block[start+1 : end]
}

// ZipPartForTest exposes a single zip entry so the end-to-end audit can inspect
// the bytes this package wrote.
func ZipPartForTest(workbook, name string) ([]byte, error) { return zipPart(workbook, name) }

// openZip opens a workbook for streaming reads.
func openZip(path string) (*zip.ReadCloser, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return zr, nil
}

// openPart returns a reader for one zip entry; the caller closes it.
func openPart(workbook, name string) (io.ReadCloser, error) {
	zr, err := zip.OpenReader(workbook)
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				zr.Close()
				return nil, err
			}
			return &partReader{rc: rc, zr: zr}, nil
		}
	}
	zr.Close()
	return nil, fmt.Errorf("压缩包中找不到 %s", name)
}

// partReader closes the whole archive when the entry is closed.
type partReader struct {
	rc io.ReadCloser
	zr *zip.ReadCloser
}

func (p *partReader) Read(b []byte) (int, error) { return p.rc.Read(b) }
func (p *partReader) Close() error {
	_ = p.rc.Close()
	return p.zr.Close()
}
