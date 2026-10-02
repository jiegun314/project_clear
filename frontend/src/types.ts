/** Shapes returned by the Go bindings. */

export interface Week {
  code: string;
  year: number;
  weekNo: number;
  start: string; // YYYY-MM-DD
}

export interface GridHeader {
  source: string;
  weekCode: string;
  weekStart: string;
  indexNames: string[];
  weeks: Week[];
  total: number;
  hasStaging: boolean;
}

export interface GridRow {
  seq: number;
  index: string[];
  weeks: string[];
  fileName: string;
  srcRow: number;
}

export interface GridResult {
  rows: GridRow[];
  total: number;
  page: number;
  pageSize: number;
}

export interface GridQuery {
  source: string;
  page: number;
  pageSize: number;
  search: string;
  sortField: string;
  sortDesc: boolean;
  filters: Record<string, string>;
}

export interface FileResult {
  name: string;
  path: string;
  size: number;
  status: string;
  rowsTotal: number;
  rowsKept: number;
  weekCode: string;
  err?: string;
}

export interface ImportResult {
  action: string;
  weekCode: string;
  weekStart: string;
  weekCodes: Week[];
  indexNames: string[];
  total: number;
  ok: number;
  failed: number;
  rowsKept: number;
  files: FileResult[];
  warnings: string[];
  durationMs: number;
}

export interface CommitResult {
  weekCode: string;
  weekStart: string;
  tableName: string;
  rowCount: number;
  fileCount: number;
  committedAt: string;
  overwrote: boolean;
}

export interface ExportResult {
  destPath: string;
  mode: string;
  rows: number;
  cols: number;
  comments: number;
  cfRows: number;
  stylesUsed: number;
  dropped: number;
  sizeBytes: number;
  preservedVba: boolean;
  durationMs: number;
}

export interface ArchiveEntry {
  weekCode: string;
  weekStart: string;
  year: number;
  weekNo: number;
  weekCodes: string[];
  indexNames: string[];
  rowCount: number;
  fileCount: number;
  batchId: number;
  committedAt: string;
  tableName: string;
}

export interface AppConfig {
  readColumns: number;
  locFilter: string;
  pageSize: number;
  headerDisplay: 'twoRow' | 'oneRow';
  exportMode: 'template' | 'clean';
  exportDir: string;
}

export interface ConfigView {
  config: AppConfig;
  path: string;
}

export interface LogEntry {
  seq: number;
  time: string;
  level: 'info' | 'success' | 'warn' | 'error';
  source: string;
  message: string;
}

export interface Status {
  hasStaging: boolean;
  weekCode: string;
  weekStart: string;
  stagedRows: number;
  archivedWeeks: number;
  archivedRows: number;
  readColumns: number;
  locFilter: string;
  pageSize: number;
  headerDisplay: string;
  exportMode: string;
  database: string;
  lastAction: string;
}

export interface AppInfo {
  name: string;
  fullName: string;
  version: string;
  dataDir: string;
  configPath: string;
  database: string;
  goVersion: string;
  platform: string;
}
