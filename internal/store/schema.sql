-- CLEAR local database schema (v5).
--
-- A week is built from two independent inputs and one integration:
--
--   dataset       one complete import of a Global or Local dataset, or the merged
--                 result of an integration. Rows live in dataset_row, so every
--                 version is a first-class snapshot.
--   integration   the week's current combination of one Global, one Local and
--                 the merged dataset produced from them.
--
-- A Global import is a *draft* while the user is still importing folder/add
-- files; finishing it flips status to ready and assigns a version. Local files
-- are a single import, so they are ready immediately. Drafts are discarded on
-- startup.

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

-- One row per completed (or in-progress) dataset.
CREATE TABLE IF NOT EXISTS dataset (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  week_code    TEXT    NOT NULL,              -- 2639
  week_start   TEXT    NOT NULL,
  kind         TEXT    NOT NULL,              -- global | local | merged
  version      INTEGER NOT NULL DEFAULT 0,    -- 0 for a draft, 1.. when ready
  status       TEXT    NOT NULL,              -- draft | ready
  row_count    INTEGER NOT NULL DEFAULT 0,
  file_count   INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  week_codes   TEXT    NOT NULL,              -- JSON array, workbook order
  index_names  TEXT    NOT NULL,              -- JSON array of A..O headers
  imported_at  TEXT    NOT NULL,
  params       TEXT,                          -- config snapshot at import time
  base_id      INTEGER,                       -- draft copied from this dataset
  merged_from_global INTEGER,
  merged_from_local  INTEGER
);
-- At most one draft overall (the open Global import), and versions are unique
-- per week and kind once a dataset is ready.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dataset_draft ON dataset(kind) WHERE status = 'draft';
CREATE UNIQUE INDEX IF NOT EXISTS idx_dataset_version
  ON dataset(week_code, kind, version) WHERE status = 'ready';
CREATE INDEX IF NOT EXISTS idx_dataset_week ON dataset(week_code, kind, status, version DESC);

-- One row of a dataset, in the MPS layout: A..O plus the week values as JSON
-- so a dataset carries its own column frame and can be compared with another
-- version without rewriting tables.
CREATE TABLE IF NOT EXISTS dataset_row (
  dataset_id INTEGER NOT NULL REFERENCES dataset(id) ON DELETE CASCADE,
  seq        INTEGER NOT NULL,
  file_id    INTEGER,
  file_name  TEXT,
  src_row    INTEGER NOT NULL DEFAULT 0,
  c1  TEXT, c2  TEXT, c3  TEXT, c4  TEXT, c5  TEXT,
  c6  TEXT, c7  TEXT, c8  TEXT, c9  TEXT, c10 TEXT,
  c11 TEXT, c12 TEXT, c13 TEXT, c14 TEXT, c15 TEXT,
  weeks      TEXT NOT NULL,                   -- JSON array, aligned to week_codes
  style_ids  TEXT NOT NULL,                   -- JSON array of interned ids
  cf_id      INTEGER,
  comments   TEXT,                            -- JSON object col -> text
  cf_colors  TEXT,                            -- JSON array, aligned to weeks
  PRIMARY KEY (dataset_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_dataset_row_dataset ON dataset_row(dataset_id, seq);

-- One workbook of a dataset, for the 已导入文件 window.
CREATE TABLE IF NOT EXISTS dataset_file (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  dataset_id INTEGER NOT NULL REFERENCES dataset(id) ON DELETE CASCADE,
  path       TEXT    NOT NULL,
  name       TEXT    NOT NULL,
  size       INTEGER NOT NULL DEFAULT 0,
  status     TEXT    NOT NULL,                  -- ok | failed
  rows_total INTEGER NOT NULL DEFAULT 0,
  rows_kept  INTEGER NOT NULL DEFAULT 0,
  week_code  TEXT,
  err        TEXT
);
CREATE INDEX IF NOT EXISTS idx_dataset_file_dataset ON dataset_file(dataset_id);

-- The week's current combination. One row per integrated week; re-integrating
-- updates it to the newest ready versions. global_id/local_id stay nullable for
-- weeks integrated by releases that predate local files.
CREATE TABLE IF NOT EXISTS integration (
  week_code     TEXT PRIMARY KEY,
  global_id     INTEGER REFERENCES dataset(id),
  local_id      INTEGER REFERENCES dataset(id),
  merged_id     INTEGER NOT NULL REFERENCES dataset(id),
  integrated_at TEXT    NOT NULL,
  params        TEXT
);

-- A stored copy of the first good source workbook, used as the export
-- template so the exported file keeps the original look.
CREATE TABLE IF NOT EXISTS template (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  week_code   TEXT    NOT NULL,
  src_name    TEXT    NOT NULL,
  stored_path TEXT    NOT NULL,
  created_at  TEXT    NOT NULL
);
