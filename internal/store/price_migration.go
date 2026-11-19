package store

// addPriceData creates the local product-price master table for a database that
// predates it. The same CREATE is in schema.sql, so on a normal upgrade this is a
// no-op; it is repeated here so the migration is self-contained and replayable.
func addPriceData(q sqlQueryer) error {
	_, err := q.Exec(`CREATE TABLE IF NOT EXISTS master_price (
	   material      TEXT PRIMARY KEY,
	   standard_cost REAL,
	   updated_at    TEXT NOT NULL
	 )`)
	return err
}
