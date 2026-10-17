package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"project_clear/internal/mps"
)

func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func hasAny(values []string) bool {
	for _, v := range values {
		if v != "" {
			return true
		}
	}
	return false
}

func resolveStyleIDsTx(tx *sql.Tx, dict *mps.StyleDict) (map[int]int, error) {
	if dict == nil {
		return map[int]int{}, nil
	}
	mapping := make(map[int]int, dict.Len())
	for id := 0; id < dict.Len(); id++ {
		sig, ok := dict.Style(id)
		if !ok {
			continue
		}
		dbID, err := internStyleTx(tx, sig)
		if err != nil {
			return nil, err
		}
		mapping[id] = dbID
	}
	return mapping, nil
}

func resolveDxfIDsTx(tx *sql.Tx, dict *mps.DxfDict) (map[int]int, error) {
	if dict == nil {
		return map[int]int{}, nil
	}
	mapping := make(map[int]int, dict.Len())
	for id := 0; id < dict.Len(); id++ {
		dxf, ok := dict.At(id)
		if !ok {
			continue
		}
		hash := hashOf([]byte(dxf))
		var stored int
		err := tx.QueryRow(`SELECT id FROM dxf_style WHERE sig_hash=?`, hash).Scan(&stored)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO dxf_style(sig_hash, payload) VALUES(?,?)`, hash, dxf)
			if err != nil {
				return nil, err
			}
			n, err := res.LastInsertId()
			if err != nil {
				return nil, err
			}
			stored = int(n)
		} else if err != nil {
			return nil, err
		}
		mapping[id] = stored
	}
	return mapping, nil
}

func internStyleTx(tx *sql.Tx, sig json.RawMessage) (int, error) {
	hash := hashOf(sig)
	var id int
	err := tx.QueryRow(`SELECT id FROM cell_style WHERE sig_hash=?`, hash).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO cell_style(sig_hash, sig) VALUES(?,?)`, hash, string(sig))
	if err != nil {
		return 0, err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func cfPatternIDTx(tx *sql.Tx, payload string) (sql.NullInt64, error) {
	if payload == "" || payload == "null" {
		return sql.NullInt64{}, nil
	}
	hash := hashOf(json.RawMessage(payload))
	var id int64
	err := tx.QueryRow(`SELECT id FROM cf_pattern WHERE sig_hash=?`, hash).Scan(&id)
	if err == nil {
		return sql.NullInt64{Int64: id, Valid: true}, nil
	}
	if err != sql.ErrNoRows {
		return sql.NullInt64{}, err
	}
	res, err := tx.Exec(`INSERT INTO cf_pattern(sig_hash, payload) VALUES(?,?)`, hash, payload)
	if err != nil {
		return sql.NullInt64{}, err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: n, Valid: true}, nil
}

// weekValue converts a stored cell into a bindable value: a number when it is
// one, the original text when it is not, and NULL when the cell was empty.
func weekValue(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return raw
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}
