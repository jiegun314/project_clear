package mps

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Relationship types that point at a worksheet's annotations. Excel keeps two
// flavours side by side: the classic note ("comments") and the modern threaded
// comment introduced in Office 365.
const (
	relComments         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	relThreadedComments = "http://schemas.microsoft.com/office/2017/10/relationships/threadedComment"
)

// threadedPlaceholder marks the legacy stub Excel writes next to a modern
// comment so that older versions show something instead of nothing.
const threadedPlaceholder = "[线程批注]"

// Comment is one cell annotation: the note text and, when the workbook records
// one, who wrote it.
type Comment struct {
	Text   string `json:"text"`
	Author string `json:"author,omitempty"`
}

// CommentMap maps a 1-based column number to the annotation on that cell.
type CommentMap map[int]Comment

// UnmarshalJSON accepts both the current {"16":{"text":..,"author":..}} shape
// and the plain {"16":"text"} shape that CLEAR 1.0 wrote, so a database filled
// before annotations were carried through storage still loads.
func (m *CommentMap) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make(CommentMap, len(raw))
	for k, v := range raw {
		col, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		var c Comment
		if err := json.Unmarshal(v, &c); err == nil {
			out[col] = c
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[col] = Comment{Text: s}
		}
	}
	*m = out
	return nil
}

// readComments collects the annotations of one worksheet, keyed by row and
// then by column. Modern threaded comments are read from their own part and
// preferred over the "[线程批注]" stub the legacy part carries for the same
// cell, because the stub is boilerplate rather than the author's words.
//
// A missing part is not an error: most workbooks have no annotations at all.
func readComments(workbook, sheet string) (map[int]CommentMap, error) {
	legacy, threaded, err := commentParts(workbook, sheet)
	if err != nil {
		return nil, err
	}
	out := map[int]CommentMap{}

	if legacy != "" {
		raw, err := zipPart(workbook, legacy)
		if err != nil {
			return nil, err
		}
		for _, c := range parseLegacyComments(raw) {
			col, row, err := cellToCoords(c.Cell)
			if err != nil || row < FirstDataRow {
				continue
			}
			text := cleanLegacyText(c.Text)
			if text == "" {
				continue
			}
			putComment(out, row, col, Comment{Text: text, Author: c.Author})
		}
	}

	if threaded != "" {
		raw, err := zipPart(workbook, threaded)
		if err != nil {
			return nil, err
		}
		persons := map[string]string{}
		if part, err := personPart(workbook, sheet); err == nil && part != "" {
			if praw, err := zipPart(workbook, part); err == nil {
				persons = parsePersons(praw)
			}
		}
		for _, c := range parseThreadedComments(raw) {
			col, row, err := cellToCoords(c.Ref)
			if err != nil || row < FirstDataRow {
				continue
			}
			text := strings.TrimSpace(c.Text)
			if text == "" {
				continue
			}
			author := persons[c.PersonID]
			// A reply is worth keeping too; fold it under the opening note so
			// nothing the planner wrote is thrown away.
			if base, ok := getComment(out, row, col); ok && c.Reply {
				text = base.Text + "\n\n" + replyPrefix(author) + text
				author = base.Author
			}
			putComment(out, row, col, Comment{Text: text, Author: author})
		}
	}

	return out, nil
}

func replyPrefix(author string) string {
	if author == "" {
		return "回复: "
	}
	return author + ": "
}

func putComment(m map[int]CommentMap, row, col int, c Comment) {
	if m[row] == nil {
		m[row] = CommentMap{}
	}
	m[row][col] = c
}

func getComment(m map[int]CommentMap, row, col int) (Comment, bool) {
	if m[row] == nil {
		return Comment{}, false
	}
	c, ok := m[row][col]
	return c, ok
}

// commentParts resolves the zip paths of the legacy and threaded comment parts
// belonging to a worksheet, either of which may be absent.
func commentParts(workbook, sheet string) (legacy, threaded string, err error) {
	sheetPart, err := SheetPartPath(workbook, sheet)
	if err != nil {
		return "", "", err
	}
	dir := path.Dir(sheetPart)
	relsPath := path.Join(dir, "_rels", path.Base(sheetPart)+".rels")
	raw, err := zipPart(workbook, relsPath)
	if err != nil {
		// A sheet without relationships simply has no annotations.
		return "", "", nil
	}
	var rels struct {
		Rel []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(raw, &rels); err != nil {
		return "", "", fmt.Errorf("解析 %s 失败: %w", relsPath, err)
	}
	for _, r := range rels.Rel {
		target := r.Target
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = path.Join(dir, target)
		}
		switch r.Type {
		case relComments:
			legacy = target
		case relThreadedComments:
			threaded = target
		}
	}
	return legacy, threaded, nil
}

// personPart finds xl/persons/person.xml, which maps the opaque person ids used
// by threaded comments onto display names.
func personPart(workbook, sheet string) (string, error) {
	sheetPart, err := SheetPartPath(workbook, sheet)
	if err != nil {
		return "", err
	}
	dir := path.Dir(sheetPart)
	relsPath := path.Join(dir, "_rels", path.Base(sheetPart)+".rels")
	raw, err := zipPart(workbook, relsPath)
	if err != nil {
		return "", nil
	}
	var rels struct {
		Rel []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(raw, &rels); err != nil {
		return "", err
	}
	for _, r := range rels.Rel {
		if !strings.Contains(r.Type, "person") {
			continue
		}
		if strings.HasPrefix(r.Target, "/") {
			return strings.TrimPrefix(r.Target, "/"), nil
		}
		return path.Join(dir, r.Target), nil
	}
	// Excel usually hangs the people list off the workbook rather than the
	// sheet; fall back to the conventional location.
	if HasPart(workbook, "xl/persons/person.xml") {
		return "xl/persons/person.xml", nil
	}
	return "", nil
}

type legacyComment struct {
	Cell   string
	Author string
	Text   string
}

func parseLegacyComments(raw []byte) []legacyComment {
	var doc struct {
		Authors struct {
			Author []string `xml:"author"`
		} `xml:"authors"`
		List struct {
			Comment []struct {
				Ref      string `xml:"ref,attr"`
				AuthorID int    `xml:"authorId,attr"`
				Text     struct {
					Inner string `xml:",innerxml"`
				} `xml:"text"`
			} `xml:"comment"`
		} `xml:"commentList"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	out := make([]legacyComment, 0, len(doc.List.Comment))
	for _, c := range doc.List.Comment {
		author := ""
		if c.AuthorID >= 0 && c.AuthorID < len(doc.Authors.Author) {
			author = strings.TrimSpace(doc.Authors.Author[c.AuthorID])
		}
		out = append(out, legacyComment{
			Cell:   c.Ref,
			Author: cleanLegacyAuthor(author),
			Text:   plainText(c.Text.Inner),
		})
	}
	return out
}

type threadedComment struct {
	Ref      string
	PersonID string
	Reply    bool
	Text     string
}

func parseThreadedComments(raw []byte) []threadedComment {
	var doc struct {
		Comment []struct {
			Ref      string `xml:"ref,attr"`
			PersonID string `xml:"personId,attr"`
			ParentID string `xml:"parentId,attr"`
			Text     struct {
				Inner string `xml:",innerxml"`
			} `xml:"text"`
		} `xml:"threadedComment"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	out := make([]threadedComment, 0, len(doc.Comment))
	for _, c := range doc.Comment {
		out = append(out, threadedComment{
			Ref:      c.Ref,
			PersonID: c.PersonID,
			Reply:    strings.TrimSpace(c.ParentID) != "",
			Text:     plainText(c.Text.Inner),
		})
	}
	return out
}

func parsePersons(raw []byte) map[string]string {
	var doc struct {
		Person []struct {
			ID          string `xml:"id,attr"`
			DisplayName string `xml:"displayName,attr"`
		} `xml:"person"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(doc.Person))
	for _, p := range doc.Person {
		if p.ID == "" {
			continue
		}
		out[p.ID] = strings.TrimSpace(p.DisplayName)
	}
	return out
}

// cleanLegacyText drops the boilerplate Excel wraps a modern comment in when
// it writes the legacy fallback, keeping whatever the author actually typed.
// The stub is not an annotation and must not be exported as one.
func cleanLegacyText(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, threadedPlaceholder) {
		return s
	}
	if i := strings.LastIndex(s, "注释:"); i >= 0 {
		return strings.TrimSpace(s[i+len("注释:"):])
	}
	return ""
}

// cleanLegacyAuthor strips the "tc={guid}" prefix Excel uses when it mirrors a
// threaded comment's author into the legacy author list.
func cleanLegacyAuthor(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "tc={") {
		if end := strings.Index(s, "}"); end >= 0 {
			return strings.TrimSpace(s[end+1:])
		}
	}
	return s
}

// plainText flattens the rich-text runs of a comment body into plain text,
// decoding character references along the way so "&#10;" becomes a real line
// break rather than four literal characters.
func plainText(inner string) string {
	if inner == "" {
		return ""
	}
	dec := xml.NewDecoder(strings.NewReader("<body>" + inner + "</body>"))
	// Comment bodies occasionally carry entities the strict parser rejects;
	// whatever was decoded before the hiccup is still worth keeping.
	dec.Strict = false
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			b.Write(cd)
		}
	}
	return strings.TrimRight(b.String(), " \t\r\n")
}

// StripThreadedComments removes the modern threaded comments anchored on or
// below fromRow from an already-saved workbook, deleting the part and its
// relationship once nothing is left.
//
// excelize reads and writes only the legacy comment part, so a template's
// threaded comments would otherwise stay pinned to cells that the merged data
// has since overwritten — annotating the wrong row rather than no row.
func StripThreadedComments(workbook, sheet string, fromRow int) error {
	_, threaded, err := commentParts(workbook, sheet)
	if err != nil || threaded == "" {
		return err
	}
	raw, err := zipPart(workbook, threaded)
	if err != nil {
		return err
	}
	kept, removed, emptied := removeThreadedRows(raw, fromRow)
	if removed == 0 {
		return nil
	}
	if !emptied {
		return ReplaceParts(workbook, map[string][]byte{threaded: kept})
	}

	sheetPart, err := SheetPartPath(workbook, sheet)
	if err != nil {
		return err
	}
	relsPath := path.Join(path.Dir(sheetPart), "_rels", path.Base(sheetPart)+".rels")
	rels, err := zipPart(workbook, relsPath)
	if err != nil {
		return err
	}
	return ReplaceParts(workbook, map[string][]byte{
		relsPath: dropRelationship(string(rels), relThreadedComments),
		threaded: nil,
	})
}

// ThreadedCommentPartForTest reports where a sheet's modern comments live, or
// "" when it has none. Exported so the end-to-end audit can prove the export
// carries no stale anchors.
func ThreadedCommentPartForTest(workbook, sheet string) (string, error) {
	_, threaded, err := commentParts(workbook, sheet)
	return threaded, err
}

// removeThreadedRows drops every threaded comment whose anchor row is at or
// below fromRow. It reports how many went and whether the part is now empty.
func removeThreadedRows(raw []byte, fromRow int) (kept []byte, removed int, emptied bool) {
	s := string(raw)
	for _, el := range extractElements(s, "<threadedComment", "</threadedComment>") {
		ref, err := attrValue(el.attrs, "ref")
		if err != nil {
			continue
		}
		if _, row, err := cellToCoords(ref); err != nil || row < fromRow {
			continue
		}
		s = strings.Replace(s, el.raw, "", 1)
		removed++
	}
	if removed == 0 {
		return nil, 0, false
	}
	if !strings.Contains(s, "<threadedComment") {
		return nil, removed, true
	}
	return []byte(s), removed, false
}

// dropRelationship removes every relationship of the given type so the part it
// pointed at can be deleted without leaving a dangling reference.
func dropRelationship(rels, relType string) []byte {
	s := rels
	for _, el := range extractElements(s, "<Relationship", "</Relationship>") {
		t, err := attrValue(el.attrs, "Type")
		if err != nil || t != relType {
			continue
		}
		s = strings.Replace(s, el.raw, "", 1)
	}
	return []byte(s)
}
