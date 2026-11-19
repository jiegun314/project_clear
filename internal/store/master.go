package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"project_clear/internal/master"
)

// MasterOverview returns the counters shown above the master-data table.
func (s *Store) MasterOverview() (master.Overview, error) {
	var out master.Overview
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_item_base`).Scan(&out.ItemCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_base_code`).Scan(&out.BaseCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT sap_code) FROM master_base_code WHERE sap_code <> ''`).
		Scan(&out.SapCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT category) FROM master_sap_category WHERE category <> ''`).
		Scan(&out.CategoryCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_base_code b
		LEFT JOIN master_sap_category c ON c.sap_code=b.sap_code
		WHERE b.sap_code='' OR c.category IS NULL OR c.category=''`).
		Scan(&out.Uncategorized); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_item_base i
		WHERE NOT EXISTS (SELECT 1 FROM master_base_code b WHERE b.base_code=i.base_code)`).
		Scan(&out.OrphanCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_sap_category c
		WHERE NOT EXISTS (SELECT 1 FROM master_base_code b WHERE b.sap_code=c.sap_code)`).
		Scan(&out.UnusedCategories); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_price`).Scan(&out.PriceCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM master_price WHERE standard_cost IS NULL`).
		Scan(&out.PriceMissing); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(changed_at),'') FROM master_audit
		WHERE action='import' AND table_name<>?`, master.TablePrice).
		Scan(&out.LastImportAt); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(changed_at),'') FROM master_audit
		WHERE action='import' AND table_name=?`, master.TablePrice).
		Scan(&out.LastPriceImportAt); err != nil {
		return out, err
	}
	rows, err := s.db.Query(`SELECT DISTINCT category FROM master_sap_category WHERE category <> '' ORDER BY category`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Categories = []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return out, err
		}
		out.Categories = append(out.Categories, c)
	}
	return out, rows.Err()
}

// MasterPage returns one page of a master table.
func (s *Store) MasterPage(q master.Query) (master.Page, error) {
	if !master.ValidTable(q.Table) {
		return master.Page{}, fmt.Errorf("未知的主数据表 %q", q.Table)
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 50
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	from, where, args, err := masterTableQuery(q)
	if err != nil {
		return master.Page{}, err
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) `+from+where, args...).Scan(&total); err != nil {
		return master.Page{}, err
	}
	offset := (q.Page - 1) * q.PageSize
	args = append(args, q.PageSize, offset)
	rows, err := s.db.Query(`SELECT `+masterSelect(q.Table)+` `+from+where+
		` ORDER BY `+masterOrder(q.Table)+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return master.Page{}, err
	}
	defer rows.Close()
	out := master.Page{Rows: []master.Row{}, Total: total, Page: q.Page, PageSize: q.PageSize}
	for rows.Next() {
		var r master.Row
		switch q.Table {
		case master.TableItem:
			var base string
			var hasBase int
			if err := rows.Scan(&r.ItemCode, &base, &r.UpdatedAt, &hasBase); err != nil {
				return out, err
			}
			r.Table, r.Key, r.BaseCode = q.Table, r.ItemCode, base
			r.Orphan = hasBase == 0
		case master.TableBase:
			var category string
			if err := rows.Scan(&r.BaseCode, &r.ManuCode, &r.SapCode, &category, &r.UpdatedAt, &r.RefCount); err != nil {
				return out, err
			}
			r.Table, r.Key, r.Category = q.Table, r.BaseCode, category
			r.Uncategorized = category == ""
		case master.TableCategory:
			if err := rows.Scan(&r.SapCode, &r.Category, &r.UpdatedAt, &r.RefCount); err != nil {
				return out, err
			}
			r.Table, r.Key = q.Table, r.SapCode
			r.Uncategorized = strings.TrimSpace(r.Category) == ""
			r.Unused = r.RefCount == 0
		case master.TablePrice:
			var cost sql.NullFloat64
			if err := rows.Scan(&r.Material, &cost, &r.UpdatedAt); err != nil {
				return out, err
			}
			if cost.Valid {
				v := cost.Float64
				r.StandardCost = &v
			}
			r.Table, r.Key = q.Table, r.Material
		}
		out.Rows = append(out.Rows, r)
	}
	return out, rows.Err()
}

func masterSelect(table string) string {
	switch table {
	case master.TableItem:
		return `i.item_code, i.base_code, i.updated_at,
			(SELECT COUNT(*) FROM master_base_code b WHERE b.base_code=i.base_code)`
	case master.TableBase:
		return `b.base_code, b.manu_code, b.sap_code, COALESCE(c.category,''), b.updated_at,
			(SELECT COUNT(*) FROM master_item_base i WHERE i.base_code=b.base_code)`
	case master.TableCategory:
		return `c.sap_code, c.category, c.updated_at,
			(SELECT COUNT(*) FROM master_base_code b WHERE b.sap_code=c.sap_code)`
	case master.TablePrice:
		return `m.material, m.standard_cost, m.updated_at`
	}
	return ""
}

func masterOrder(table string) string {
	switch table {
	case master.TableItem:
		return `item_code`
	case master.TableBase:
		return `b.base_code`
	case master.TableCategory:
		return `c.sap_code`
	case master.TablePrice:
		return `m.material`
	}
	return `1`
}

func masterTableQuery(q master.Query) (from, where string, args []any, err error) {
	search := strings.TrimSpace(q.Search)
	like := "%" + search + "%"
	switch q.Table {
	case master.TableItem:
		from = `FROM master_item_base i`
		where = ` WHERE 1=1`
		if search != "" {
			where += ` AND (i.item_code LIKE ? OR i.base_code LIKE ?)`
			args = append(args, like, like)
		}
		if q.Filter == "orphan" {
			where += ` AND NOT EXISTS (SELECT 1 FROM master_base_code b WHERE b.base_code=i.base_code)`
		}
	case master.TableBase:
		from = `FROM master_base_code b LEFT JOIN master_sap_category c ON c.sap_code=b.sap_code`
		where = ` WHERE 1=1`
		if search != "" {
			where += ` AND (b.base_code LIKE ? OR b.manu_code LIKE ? OR b.sap_code LIKE ? OR COALESCE(c.category,'') LIKE ?)`
			args = append(args, like, like, like, like)
		}
		if q.Filter == "uncategorized" {
			where += ` AND (b.sap_code='' OR c.category IS NULL OR c.category='')`
		}
	case master.TableCategory:
		from = `FROM master_sap_category c`
		where = ` WHERE 1=1`
		if search != "" {
			where += ` AND (c.sap_code LIKE ? OR c.category LIKE ?)`
			args = append(args, like, like)
		}
		if q.Filter == "unused" {
			where += ` AND NOT EXISTS (SELECT 1 FROM master_base_code b WHERE b.sap_code=c.sap_code)`
		}
	case master.TablePrice:
		from = `FROM master_price m`
		where = ` WHERE 1=1`
		if search != "" {
			where += ` AND m.material LIKE ?`
			args = append(args, like)
		}
		if q.Filter == "unpriced" {
			where += ` AND m.standard_cost IS NULL`
		}
	default:
		return "", "", nil, fmt.Errorf("未知的主数据表 %q", q.Table)
	}
	return from, where, args, nil
}

func existsQ(q sqlQueryer, query string, args ...any) bool {
	var n int
	return q.QueryRow(query, args...).Scan(&n) == nil
}

// ReplaceMasterData replaces all three tables with the workbook contents. It is
// one transaction: a failure leaves the previous master data untouched.
func (s *Store) ReplaceMasterData(w *master.Workbook) error {
	if w == nil {
		return fmt.Errorf("没有可导入的主数据")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := Now()
	for _, table := range []string{"master_item_base", "master_base_code", "master_sap_category"} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return err
		}
	}
	itemStmt, err := tx.Prepare(`INSERT INTO master_item_base(item_code,base_code,updated_at) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer itemStmt.Close()
	for _, r := range w.Items {
		if _, err := itemStmt.Exec(r.ItemCode, r.BaseCode, now); err != nil {
			return fmt.Errorf("写入 Item %s 失败: %w", r.ItemCode, err)
		}
	}
	baseStmt, err := tx.Prepare(`INSERT INTO master_base_code(base_code,manu_code,sap_code,updated_at) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer baseStmt.Close()
	for _, r := range w.Bases {
		if _, err := baseStmt.Exec(r.BaseCode, r.ManuCode, r.SapCode, now); err != nil {
			return fmt.Errorf("写入 Base %s 失败: %w", r.BaseCode, err)
		}
	}
	catStmt, err := tx.Prepare(`INSERT INTO master_sap_category(sap_code,category,updated_at) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer catStmt.Close()
	for _, r := range w.Categories {
		if _, err := catStmt.Exec(r.SapCode, r.Category, now); err != nil {
			return fmt.Errorf("写入 SAP %s 失败: %w", r.SapCode, err)
		}
	}
	after, _ := json.Marshal(map[string]any{
		"path": w.Name, "items": len(w.Items), "bases": len(w.Bases), "categories": len(w.Categories),
	})
	if _, err := tx.Exec(`INSERT INTO master_audit(table_name,key_value,action,before_json,after_json,changed_at)
		VALUES('all','', 'import','',?,?)`, string(after), now); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplacePriceData replaces the product-price table in one transaction. The code
// mapping tables are intentionally untouched.
func (s *Store) ReplacePriceData(w *master.PriceWorkbook) error {
	if w == nil {
		return fmt.Errorf("没有可导入的价格数据")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := Now()
	if _, err := tx.Exec(`DELETE FROM master_price`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO master_price(material,standard_cost,updated_at) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range w.Prices {
		var cost any
		if p.StandardCost != nil {
			cost = *p.StandardCost
		}
		if _, err := stmt.Exec(p.Material, cost, now); err != nil {
			return fmt.Errorf("写入价格 %s 失败: %w", p.Material, err)
		}
	}
	after, _ := json.Marshal(map[string]any{
		"path": w.Name, "prices": len(w.Prices), "missingCosts": w.MissingCosts,
	})
	if _, err := tx.Exec(`INSERT INTO master_audit(table_name,key_value,action,before_json,after_json,changed_at)
		VALUES(?, '', 'import','',?,?)`, master.TablePrice, string(after), now); err != nil {
		return err
	}
	return tx.Commit()
}

// MasterRowInput is the normalized form used by SaveMasterRow.
func (s *Store) SaveMasterRow(in master.Input) error {
	if !master.ValidTable(in.Table) {
		return fmt.Errorf("未知的主数据表 %q", in.Table)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := Now()
	orig := strings.TrimSpace(in.OriginalKey)
	var before string
	var beforeFound bool
	if orig != "" {
		before, beforeFound, err = masterSnapshot(tx, in.Table, orig)
		if err != nil {
			return err
		}
	}
	switch in.Table {
	case master.TableItem:
		item := strings.TrimSpace(in.ItemCode)
		base := strings.TrimSpace(in.BaseCode)
		if item == "" || base == "" {
			return fmt.Errorf("Item Code 和 Base Code 不能为空")
		}
		if orig != "" && orig != item {
			if existsQ(tx, `SELECT 1 FROM master_item_base WHERE item_code=?`, item) {
				return fmt.Errorf("Item Code %s 已存在", item)
			}
			if _, err := tx.Exec(`DELETE FROM master_item_base WHERE item_code=?`, orig); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO master_item_base(item_code,base_code,updated_at) VALUES(?,?,?)
			ON CONFLICT(item_code) DO UPDATE SET base_code=excluded.base_code, updated_at=excluded.updated_at`,
			item, base, now)
		if err != nil {
			return err
		}
		in.ItemCode = item
		in.BaseCode = base
	case master.TableBase:
		base := strings.TrimSpace(in.BaseCode)
		if base == "" {
			return fmt.Errorf("Base Code 不能为空")
		}
		if orig != "" && orig != base {
			if existsQ(tx, `SELECT 1 FROM master_base_code WHERE base_code=?`, base) {
				return fmt.Errorf("Base Code %s 已存在", base)
			}
			if existsQ(tx, `SELECT 1 FROM master_item_base WHERE base_code=?`, orig) {
				return fmt.Errorf("Base Code %s 仍被 Item 使用，不能修改主键", orig)
			}
			if _, err := tx.Exec(`DELETE FROM master_base_code WHERE base_code=?`, orig); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO master_base_code(base_code,manu_code,sap_code,updated_at) VALUES(?,?,?,?)
			ON CONFLICT(base_code) DO UPDATE SET manu_code=excluded.manu_code, sap_code=excluded.sap_code, updated_at=excluded.updated_at`,
			base, strings.TrimSpace(in.ManuCode), strings.TrimSpace(in.SapCode), now)
		if err != nil {
			return err
		}
		in.BaseCode = base
	case master.TableCategory:
		sap := strings.TrimSpace(in.SapCode)
		if sap == "" {
			return fmt.Errorf("SAP Code 不能为空")
		}
		if orig != "" && orig != sap {
			if existsQ(tx, `SELECT 1 FROM master_sap_category WHERE sap_code=?`, sap) {
				return fmt.Errorf("SAP Code %s 已存在", sap)
			}
			if existsQ(tx, `SELECT 1 FROM master_base_code WHERE sap_code=?`, orig) {
				return fmt.Errorf("SAP Code %s 仍被 Base 使用，不能修改主键", orig)
			}
			if _, err := tx.Exec(`DELETE FROM master_sap_category WHERE sap_code=?`, orig); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO master_sap_category(sap_code,category,updated_at) VALUES(?,?,?)
			ON CONFLICT(sap_code) DO UPDATE SET category=excluded.category, updated_at=excluded.updated_at`,
			sap, strings.TrimSpace(in.Category), now)
		if err != nil {
			return err
		}
		in.SapCode = sap
	case master.TablePrice:
		material := strings.TrimSpace(in.Material)
		if material == "" {
			return fmt.Errorf("Material 不能为空")
		}
		if orig != "" && orig != material {
			if existsQ(tx, `SELECT 1 FROM master_price WHERE material=?`, material) {
				return fmt.Errorf("Material %s 已存在", material)
			}
			if _, err := tx.Exec(`DELETE FROM master_price WHERE material=?`, orig); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO master_price(material,standard_cost,updated_at) VALUES(?,?,?)
			ON CONFLICT(material) DO UPDATE SET standard_cost=excluded.standard_cost, updated_at=excluded.updated_at`,
			material, in.StandardCost, now)
		if err != nil {
			return err
		}
		in.Material = material
	}
	after, _ := json.Marshal(in)
	action := "update"
	if orig == "" {
		action = "create"
	}
	if _, err := tx.Exec(`INSERT INTO master_audit(table_name,key_value,action,before_json,after_json,changed_at)
		VALUES(?,?,?,?,?,?)`, in.Table, strings.TrimSpace(in.ItemCode+in.BaseCode+in.SapCode+in.Material), action, before, string(after), now); err != nil {
		return err
	}
	if orig != "" && !beforeFound {
		return fmt.Errorf("要修改的记录已不存在")
	}
	return tx.Commit()
}

func masterSnapshot(q sqlQueryer, table, key string) (string, bool, error) {
	var data map[string]any
	switch table {
	case master.TableItem:
		var item, base, at string
		err := q.QueryRow(`SELECT item_code,base_code,updated_at FROM master_item_base WHERE item_code=?`, key).
			Scan(&item, &base, &at)
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		data = map[string]any{"itemCode": item, "baseCode": base, "updatedAt": at}
	case master.TableBase:
		var base, manu, sap, at string
		err := q.QueryRow(`SELECT base_code,manu_code,sap_code,updated_at FROM master_base_code WHERE base_code=?`, key).
			Scan(&base, &manu, &sap, &at)
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		data = map[string]any{"baseCode": base, "manuCode": manu, "sapCode": sap, "updatedAt": at}
	case master.TableCategory:
		var sap, category, at string
		err := q.QueryRow(`SELECT sap_code,category,updated_at FROM master_sap_category WHERE sap_code=?`, key).
			Scan(&sap, &category, &at)
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		data = map[string]any{"sapCode": sap, "category": category, "updatedAt": at}
	case master.TablePrice:
		var material string
		var cost sql.NullFloat64
		var at string
		err := q.QueryRow(`SELECT material,standard_cost,updated_at FROM master_price WHERE material=?`, key).
			Scan(&material, &cost, &at)
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		var value any
		if cost.Valid {
			value = cost.Float64
		}
		data = map[string]any{"material": material, "standardCost": value, "updatedAt": at}
	default:
		return "", false, fmt.Errorf("未知的主数据表 %q", table)
	}
	raw, _ := json.Marshal(data)
	return string(raw), true, nil
}

// DeleteMasterRow removes one mapping, refusing a delete that would leave
// referencing rows behind (unless the caller is deleting a leaf Item).
func (s *Store) DeleteMasterRow(table, key string) error {
	if !master.ValidTable(table) {
		return fmt.Errorf("未知的主数据表 %q", table)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("主键不能为空")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	before, found, err := masterSnapshot(tx, table, key)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("记录不存在")
	}
	switch table {
	case master.TableItem:
		if _, err := tx.Exec(`DELETE FROM master_item_base WHERE item_code=?`, key); err != nil {
			return err
		}
	case master.TableBase:
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM master_item_base WHERE base_code=?`, key).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("Base Code %s 仍被 %d 个 Item 使用，不能删除", key, n)
		}
		if _, err := tx.Exec(`DELETE FROM master_base_code WHERE base_code=?`, key); err != nil {
			return err
		}
	case master.TableCategory:
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM master_base_code WHERE sap_code=?`, key).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("SAP Code %s 仍被 %d 个 Base 使用，不能删除", key, n)
		}
		if _, err := tx.Exec(`DELETE FROM master_sap_category WHERE sap_code=?`, key); err != nil {
			return err
		}
	case master.TablePrice:
		if _, err := tx.Exec(`DELETE FROM master_price WHERE material=?`, key); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO master_audit(table_name,key_value,action,before_json,after_json,changed_at)
		VALUES(?,?,?,?, '', ?)`, table, key, "delete", before, Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// MasterLookup returns the Item → Base → SAP → Category chain together with the
// price attached to the SAP code. It is deliberately one query: the store has a
// single SQLite connection, so issuing the joins separately while a cursor is
// open would deadlock.
func (s *Store) MasterLookup() (map[string]master.Lookup, error) {
	rows, err := s.db.Query(`SELECT i.item_code, i.base_code,
		COALESCE(b.sap_code,''), COALESCE(c.category,''), p.standard_cost
		FROM master_item_base i
		LEFT JOIN master_base_code b ON b.base_code = i.base_code
		LEFT JOIN master_sap_category c ON c.sap_code = b.sap_code
		LEFT JOIN master_price p ON p.material = b.sap_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]master.Lookup{}
	for rows.Next() {
		var l master.Lookup
		var cost sql.NullFloat64
		if err := rows.Scan(&l.ItemCode, &l.BaseCode, &l.SapCode, &l.Category, &cost); err != nil {
			return nil, err
		}
		if cost.Valid {
			v := cost.Float64
			l.StandardCost = &v
		}
		out[l.ItemCode] = l
	}
	return out, rows.Err()
}
