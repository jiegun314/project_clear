package mps

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// excelize's GetRows is convenient but spends most of its time on per-cell
// number-format and style resolution that this application never uses: the
// staging layer stores raw cell text. A streaming pass over the sheet XML cuts
// the read of a 4 MB workbook from ~13 s to well under a second, which is the
// difference between a 3 minute import and a 20 second one.

const (
	nsMain = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
)

// valueReader streams a worksheet and yields raw cell text.
type valueReader struct {
	dec *xml.Decoder
	rc  io.Closer
}

// Close releases the worksheet part.
func (v *valueReader) Close() error {
	if v.rc == nil {
		return nil
	}
	return v.rc.Close()
}

// cellValue is one raw cell.
type cellValue struct {
	col   int
	row   int
	typ   string
	value string
	// inline carries the text of an inline string cell.
	inline string
}

// text returns the cell's raw text, resolving shared and inline strings.
func (c cellValue) text(shared []string) string {
	if c.inline != "" {
		return c.inline
	}
	switch c.typ {
	case "s":
		if i, err := strconv.Atoi(strings.TrimSpace(c.value)); err == nil && i >= 0 && i < len(shared) {
			return shared[i]
		}
		return ""
	case "str":
		return c.value
	case "inlineStr":
		return c.inline
	default:
		return c.value
	}
}

// newValueReader opens the worksheet part of a workbook for streaming.
func newValueReader(workbook, sheet string) (*valueReader, error) {
	part, err := SheetPartPath(workbook, sheet)
	if err != nil {
		return nil, err
	}
	rc, err := openPart(workbook, part)
	if err != nil {
		return nil, err
	}
	return &valueReader{dec: xml.NewDecoder(rc), rc: rc}, nil
}

// sharedStrings reads the workbook's string table, skipping empty cells.
func sharedStrings(workbook string) ([]string, error) {
	rc, err := openPart(workbook, "xl/sharedStrings.xml")
	if err != nil {
		return nil, nil // the workbook may legitimately have none
	}
	defer rc.Close()
	var out []string
	dec := xml.NewDecoder(rc)
	var inItem bool
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "si" {
				inItem = true
				text.Reset()
			}
		case xml.CharData:
			if inItem {
				text.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "si" {
				out = append(out, text.String())
				inItem = false
			}
		}
	}
	return out, nil
}

// readRows streams the cells of every data row, calling visit with the row
// number and the cells that fall inside [firstCol, lastCol].
func (v *valueReader) readRows(shared []string, firstRow, firstCol, lastCol int, visit func(row int, cells []string) error) error {
	dec := v.dec
	var (
		inSheetData bool
		inRow       bool
		inCell      bool
		inInline    bool
		rowNum      int
		cell        cellValue
		inlineText  strings.Builder
		cells       []string
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sheetData":
				inSheetData = true
			case "row":
				if !inSheetData {
					continue
				}
				rowNum = attrInt(t, "r")
				cells = nil
				inRow = true
			case "c":
				if !inRow {
					continue
				}
				ref := attrString(t, "r")
				col, err := columnOf(ref)
				if err != nil {
					inCell = false
					continue
				}
				cell = cellValue{col: col, row: rowNum, typ: attrString(t, "t")}
				inCell = true
				inInline = false
				inlineText.Reset()
			case "v":
				if inCell {
					// DecodeElement writes character data into *[]byte; a
					// strings.Builder is not a type the XML decoder knows.
					var buf []byte
					if err := dec.DecodeElement(&buf, &t); err != nil {
						return err
					}
					cell.value = string(buf)
				}
			case "is":
				if inCell {
					inInline = true
				}
			case "t":
				if inInline {
					var buf []byte
					if err := dec.DecodeElement(&buf, &t); err != nil {
						return err
					}
					inlineText.WriteString(trimSpaceBytes(buf))
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v", "is":
				// handled inline
			case "c":
				if inCell {
					cell.inline = inlineText.String()
					if cell.col >= firstCol && cell.col <= lastCol {
						for len(cells) < cell.col-firstCol+1 {
							cells = append(cells, "")
						}
						cells[cell.col-firstCol] = cell.text(shared)
					}
				}
				inCell = false
				inInline = false
			case "row":
				if inRow && rowNum >= firstRow && cells != nil {
					if err := visit(rowNum, cells); err != nil {
						return err
					}
				}
				inRow = false
				cells = nil
			case "sheetData":
				// The rest of the part holds no cell values.
				return nil
			}
		}
	}
}

func trimSpaceBytes(b []byte) string {
	return strings.TrimSpace(string(b))
}

func attrString(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func attrInt(t xml.StartElement, name string) int {
	v := attrString(t, name)
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// columnOf converts an A1-style reference to a 1-based column number. Excel
// writes references in upper case, but a lower-case one is still a valid
// reference and must not be dropped silently.
func columnOf(ref string) (int, error) {
	ref = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(ref), "$"))
	if ref == "" {
		return 0, fmt.Errorf("空引用")
	}
	n := 0
	i := 0
	for ; i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z'; i++ {
		n = n*26 + int(ref[i]-'A'+1)
	}
	if n == 0 {
		return 0, fmt.Errorf("非法引用 %q", ref)
	}
	return n, nil
}
