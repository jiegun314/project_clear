package store

import (
	"database/sql"
	"encoding/json"
)

// StyleEntry is a persisted cell style signature.
type StyleEntry struct {
	ID  int
	Sig json.RawMessage
}

// LoadStyles returns every interned style so a batch can reuse the ids that
// were already stored.
func (s *Store) LoadStyles() ([]StyleEntry, error) {
	rows, err := s.db.Query(`SELECT id, sig FROM cell_style ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StyleEntry
	for rows.Next() {
		var e StyleEntry
		var sig string
		if err := rows.Scan(&e.ID, &sig); err != nil {
			return nil, err
		}
		e.Sig = json.RawMessage(sig)
		out = append(out, e)
	}
	return out, rows.Err()
}

// CFPattern returns a stored conditional-format program.
func (s *Store) CFPattern(id int64) (string, error) {
	var p string
	err := s.db.QueryRow(`SELECT payload FROM cf_pattern WHERE id=?`, id).Scan(&p)
	if err == nil {
		return p, nil
	}
	return "", err
}

// DxfStyle is one stored differential format: the id rules refer to, and the
// <dxf> block itself.
type DxfStyle struct {
	ID  int
	XML string
}

// LoadDxfStyles returns every stored differential format, in id order.
func (s *Store) LoadDxfStyles() ([]DxfStyle, error) {
	rows, err := s.db.Query(`SELECT id, payload FROM dxf_style ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DxfStyle{}
	for rows.Next() {
		var id int
		var payload string
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, err
		}
		out = append(out, DxfStyle{ID: id, XML: payload})
	}
	return out, rows.Err()
}

// LoadCFPatterns returns every interned conditional-format program. They are
// read in one go because FetchExportRows needs them while a cursor is open,
// and the connection pool deliberately holds a single writer.
func (s *Store) LoadCFPatterns() (map[int64]string, error) {
	rows, err := s.db.Query(`SELECT id, payload FROM cf_pattern`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// Template is a stored copy of a source workbook used for export.
type Template struct {
	ID         int64
	WeekCode   string
	SrcName    string
	StoredPath string
	CreatedAt  string
}

// SaveTemplate registers a stored template copy.
func (s *Store) SaveTemplate(weekCode, srcName, storedPath string) (*Template, error) {
	// One template per week: a later batch for the same week replaces it.
	if _, err := s.db.Exec(`DELETE FROM template WHERE week_code=?`, weekCode); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO template(week_code, src_name, stored_path, created_at) VALUES(?,?,?,?)`,
		weekCode, srcName, storedPath, Now())
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Template{ID: id, WeekCode: weekCode, SrcName: srcName, StoredPath: storedPath, CreatedAt: Now()}, nil
}

// TemplateFor returns the stored template for a week.
func (s *Store) TemplateFor(weekCode string) (*Template, error) {
	var t Template
	err := s.db.QueryRow(
		`SELECT id, week_code, src_name, stored_path, created_at FROM template WHERE week_code=?`, weekCode).
		Scan(&t.ID, &t.WeekCode, &t.SrcName, &t.StoredPath, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
