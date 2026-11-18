package store

// addMasterData creates the master-data tables for a database that predates
// them. The same CREATE statements are in schema.sql, which runs before this
// migration, so on a normal upgrade they are no-ops; keeping them here makes the
// migration replayable and self-contained.
func addMasterData(q sqlQueryer) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS master_item_base (
		   item_code  TEXT PRIMARY KEY,
		   base_code  TEXT NOT NULL,
		   updated_at TEXT NOT NULL
		 )`,
		`CREATE INDEX IF NOT EXISTS idx_master_item_base_base ON master_item_base(base_code)`,
		`CREATE TABLE IF NOT EXISTS master_base_code (
		   base_code  TEXT PRIMARY KEY,
		   manu_code  TEXT NOT NULL DEFAULT '',
		   sap_code   TEXT NOT NULL DEFAULT '',
		   updated_at TEXT NOT NULL
		 )`,
		`CREATE INDEX IF NOT EXISTS idx_master_base_code_sap ON master_base_code(sap_code)`,
		`CREATE TABLE IF NOT EXISTS master_sap_category (
		   sap_code   TEXT PRIMARY KEY,
		   category   TEXT NOT NULL DEFAULT '',
		   updated_at TEXT NOT NULL
		 )`,
		`CREATE TABLE IF NOT EXISTS master_audit (
		   id          INTEGER PRIMARY KEY AUTOINCREMENT,
		   table_name  TEXT NOT NULL,
		   key_value   TEXT NOT NULL,
		   action      TEXT NOT NULL,
		   before_json TEXT NOT NULL DEFAULT '',
		   after_json  TEXT NOT NULL DEFAULT '',
		   changed_at  TEXT NOT NULL
		 )`,
		`CREATE INDEX IF NOT EXISTS idx_master_audit_time ON master_audit(changed_at)`,
	}
	for _, stmt := range stmts {
		if _, err := q.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
