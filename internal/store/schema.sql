-- CLEAR local database schema.
--
-- Two-stage design, as specified: an import or add action fills the staging
-- table so the merged data can be reviewed immediately, and pressing 整合
-- promotes it into a permanent per-week table named after the first week code
-- (2639 -> data_2639), overwriting any previous copy of that week.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS app_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- Interned cell styles. Signatures are JSON encodings of excelize styles;
-- there are only a few dozen distinct ones across a batch of workbooks.
CREATE TABLE IF NOT EXISTS cell_style (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  sig_hash TEXT NOT NULL UNIQUE,
  sig     TEXT NOT NULL
);

-- Interned per-row conditional-format programs.
CREATE TABLE IF NOT EXISTS cf_pattern (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  sig_hash TEXT NOT NULL UNIQUE,
  payload  TEXT NOT NULL
);

-- Interned differential formats (<dxf> blocks) that conditional-format rules
-- paint with. Each source workbook numbers its own, so ids are reassigned for
-- the batch and mapped back to the export's table on the way out.
CREATE TABLE IF NOT EXISTS dxf_style (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  sig_hash TEXT NOT NULL UNIQUE,
  payload  TEXT NOT NULL
);

-- One row per import/add action.
CREATE TABLE IF NOT EXISTS batch (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at     TEXT    NOT NULL,
  action         TEXT    NOT NULL,              -- import | add
  week_code      TEXT    NOT NULL,
  week_start     TEXT    NOT NULL,
  week_codes     TEXT    NOT NULL,              -- JSON array, workbook order
  index_names    TEXT    NOT NULL,              -- JSON array of A..O headers
  status         TEXT    NOT NULL,              -- staging | committed
  file_total     INTEGER NOT NULL DEFAULT 0,
  file_ok        INTEGER NOT NULL DEFAULT 0,
  file_fail      INTEGER NOT NULL DEFAULT 0,
  row_total      INTEGER NOT NULL DEFAULT 0,
  row_kept       INTEGER NOT NULL DEFAULT 0,
  param_snapshot TEXT,
  template_id    INTEGER
);

CREATE TABLE IF NOT EXISTS batch_file (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id   INTEGER NOT NULL REFERENCES batch(id) ON DELETE CASCADE,
  path       TEXT    NOT NULL,
  name       TEXT    NOT NULL,
  size       INTEGER NOT NULL DEFAULT 0,
  status     TEXT    NOT NULL,                  -- ok | failed
  rows_total INTEGER NOT NULL DEFAULT 0,
  rows_kept  INTEGER NOT NULL DEFAULT 0,
  week_code  TEXT,
  err        TEXT
);
CREATE INDEX IF NOT EXISTS idx_batch_file_batch ON batch_file(batch_id);

-- Staging rows. Exactly one batch is 'staging' at a time and its rows are
-- replaced on every import or add.
CREATE TABLE IF NOT EXISTS stg_row (
  batch_id  INTEGER NOT NULL REFERENCES batch(id) ON DELETE CASCADE,
  seq       INTEGER NOT NULL,
  file_id   INTEGER,
  file_name TEXT,
  src_row   INTEGER NOT NULL DEFAULT 0,
  c1        TEXT, c2  TEXT, c3  TEXT, c4  TEXT, c5  TEXT,
  c6        TEXT, c7  TEXT, c8  TEXT, c9  TEXT, c10 TEXT,
  c11       TEXT, c12 TEXT, c13 TEXT, c14 TEXT, c15 TEXT,
  weeks     TEXT    NOT NULL,                   -- JSON array, aligned to week_codes
  style_ids TEXT    NOT NULL,                   -- JSON array of interned ids
  cf_id     INTEGER,
  comments  TEXT,                                -- JSON object col -> text
  cf_colors TEXT,                                -- JSON array, aligned to weeks (条件格式算出的底色)
  PRIMARY KEY (batch_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_stg_row_batch ON stg_row(batch_id, seq);

-- A stored copy of the first good source workbook, used as the export
-- template so the exported file keeps the original look.
CREATE TABLE IF NOT EXISTS template (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  week_code   TEXT    NOT NULL,
  src_name    TEXT    NOT NULL,
  stored_path TEXT    NOT NULL,
  created_at  TEXT    NOT NULL
);

-- Registry of committed weeks.
CREATE TABLE IF NOT EXISTS archive (
  week_code      TEXT PRIMARY KEY,
  week_start     TEXT    NOT NULL,
  week_codes     TEXT    NOT NULL,
  index_names    TEXT    NOT NULL,
  row_count      INTEGER NOT NULL,
  file_count     INTEGER NOT NULL,
  batch_id       INTEGER,
  committed_at   TEXT    NOT NULL,
  template_id    INTEGER,
  param_snapshot TEXT,
  table_name     TEXT    NOT NULL
);
