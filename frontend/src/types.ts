/** Shapes returned by the Go bindings. */

export interface Week {
  code: string;
  year: number;
  weekNo: number;
  start: string; // YYYY-MM-DD
}

export interface GridHeader {
  datasetId: number;
  kind: string;
  version: number;
  weekCode: string;
  weekStart: string;
  indexNames: string[];
  weeks: Week[];
  total: number;
  expired: boolean;
}

export interface GridRow {
  seq: number;
  index: string[];
  weeks: string[];
  /** Aligned to weeks: the source cell's fill colour and note, if any. */
  weekMeta: CellMeta[];
  fileName: string;
  srcRow: number;
}

export interface CellMeta {
  color?: string;
  comment?: string;
  author?: string;
}

export interface GridResult {
  rows: GridRow[];
  total: number;
  page: number;
  pageSize: number;
}

export interface GridQuery {
  datasetId: number;
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
  /** Draft workbooks this add replaced (matched by file name). */
  replaced?: string[];
  /** True for a Global import that is not finished yet. */
  draft: boolean;
  datasetId: number;
  localRows: number;
  version: number;
  expired: boolean;
  rowCount: number;
  fileCount: number;
}

export interface FinishResult {
  datasetId: number;
  weekCode: string;
  weekStart: string;
  version: number;
  rowCount: number;
  fileCount: number;
  failedCount: number;
  weekCodes: Week[];
  indexNames: string[];
  importedAt: string;
  expired: boolean;
  integrationAt?: string;
}

/** One workbook of the current draft/Global or Local import. */
export interface FileWithDataset {
  name: string;
  path: string;
  size: number;
  status: string;
  rowsTotal: number;
  rowsKept: number;
  weekCode: string;
  err?: string;
  /** "Global（导入中）" | "Global" | "Local" */
  dataset: string;
  version: number;
}

export interface FilesView {
  files: FileWithDataset[];
  hasDraft: boolean;
  fileCount: number;
  rowCount: number;
  failedCount: number;
  note?: string;
}

export interface CommitResult {
  weekCode: string;
  weekStart: string;
  mergedId: number;
  globalId: number;
  localId: number;
  mergedRows: number;
  globalRows: number;
  localRows: number;
  globalVersion: number;
  localVersion: number;
  mergedVersion: number;
  fileCount: number;
  committedAt: string;
  overwrote: boolean;
  wasExpired: boolean;
}

export interface DiscardWeekResult {
  weekCode: string;
  datasets: number;
}

export interface ExportResult {
  destPath: string;
  datasetId: number;
  weekCode: string;
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
  sheets: number;
  expired: boolean;
}

export interface ArchiveEntry {
  weekCode: string;
  weekStart: string;
  year: number;
  weekNo: number;
  dataset: string;
  datasetId: number;
  version: number;
  rowCount: number;
  fileCount: number;
  committedAt: string;
  expired: boolean;
}

export interface AppConfig {
  configVersion: number;
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
  hasDraft: boolean;
  draftId: number;
  draftWeek: string;
  draftRows: number;
  draftFiles: number;
  draftFailed: number;

  hasGlobal: boolean;
  globalId: number;
  globalWeek: string;
  globalRows: number;
  globalFiles: number;
  globalVersion: number;
  globalAt: string;

  hasLocal: boolean;
  localId: number;
  localWeek: string;
  localRows: number;
  localFiles: number;
  localVersion: number;
  localAt: string;
  localFileName: string;

  hasIntegration: boolean;
  integrationId: number;
  integrationWeek: string;
  integrationRows: number;
  integrationAt: string;
  integrationExpired: boolean;

  activeWeek: string;
  unintegrated: boolean;
  canDiscardWeek: boolean;

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
