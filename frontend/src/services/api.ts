/**
 * Thin typed wrapper over the bindings Wails injects at `window.go.main.App`.
 *
 * The generated `wailsjs/` folder is produced by the build and is not present
 * during a plain `tsc` run, so the surface is declared here instead. That keeps
 * `npm run build` independent of whether bindings have been generated yet.
 */
import type {
  AppConfig,
  AppInfo,
  ArchiveEntry,
  CommitResult,
  ConfigView,
  DiscardWeekResult,
  ExportResult,
  FilesView,
  FinishResult,
  GridHeader,
  GridQuery,
  GridResult,
  ImportResult,
  LogEntry,
  Status,
} from '../types';

interface AppBindings {
  GetAppInfo(): Promise<AppInfo>;
  GetStatus(): Promise<Status>;
  ImportFolder(): Promise<ImportResult | null>;
  AddFiles(): Promise<ImportResult | null>;
  FinishGlobalMPS(confirmFailures: boolean): Promise<FinishResult>;
  ImportLocalMPS(): Promise<ImportResult | null>;
  Commit(weekLimit: number): Promise<CommitResult>;
  DiscardDraft(): Promise<ImportResult>;
  DiscardWeekImports(weekCode: string): Promise<DiscardWeekResult>;
  GetFiles(): Promise<FilesView>;
  Export(datasetId: number, mode: string): Promise<ExportResult | null>;
  ExportBundle(weekCode: string): Promise<ExportResult | null>;
  RevealExport(path: string): Promise<void>;
  QueryData(q: GridQuery): Promise<GridResult>;
  GetGridHeader(datasetId: number): Promise<GridHeader>;
  GetArchive(): Promise<ArchiveEntry[]>;
  GetYears(): Promise<number[]>;
  GetWeeks(year: number): Promise<number[]>;
  GetConfig(): Promise<ConfigView>;
  SaveConfig(c: AppConfig): Promise<string[]>;
  ResetConfig(): Promise<ConfigView>;
  GetLogs(limit: number): Promise<LogEntry[]>;
  ClearLogs(): Promise<void>;
  Quit(): Promise<void>;
}

interface RuntimeBindings {
  EventsOn(name: string, cb: (...data: unknown[]) => void): () => void;
  EventsOff(name: string): void;
  WindowHide(): void;
  Quit(): void;
  BrowserOpenURL(url: string): void;
}

declare global {
  interface Window {
    go?: { main?: { App?: AppBindings } };
    runtime?: RuntimeBindings;
  }
}

function app(): AppBindings {
  const bindings = window.go?.main?.App;
  if (!bindings) {
    throw new Error('Wails 后端未就绪：请通过 CLEAR 桌面程序运行，而不是浏览器。');
  }
  return bindings;
}

/** True when the page is running inside the Wails shell. */
export const hasBackend = (): boolean => Boolean(window.go?.main?.App);

export const api = {
  getAppInfo: () => app().GetAppInfo(),
  getStatus: () => app().GetStatus(),
  importFolder: () => app().ImportFolder(),
  /** Adds files to the open Global draft (copy-on-write when it is complete). */
  addFiles: () => app().AddFiles(),
  /** Completes the Global draft; failures have to be acknowledged. */
  finishGlobalMPS: (confirmFailures: boolean) => app().FinishGlobalMPS(confirmFailures),
  /** The Local workbook is a complete dataset on its own. */
  importLocalMPS: () => app().ImportLocalMPS(),
  commit: (weekLimit: number) => app().Commit(weekLimit),
  /** Discards the open Global draft only. */
  discardDraft: () => app().DiscardDraft(),
  /** Discards the week's un-integrated exports; history is untouched. */
  discardWeekImports: (weekCode: string) => app().DiscardWeekImports(weekCode),
  getFiles: () => app().GetFiles(),
  /** One dataset as the single-sheet report; mode is "", "clean" or "template". */
  exportDataset: (datasetId: number, mode: string) => app().Export(datasetId, mode),
  /** The integrated week's MPS / Local / Global workbook. */
  exportBundle: (weekCode: string) => app().ExportBundle(weekCode),
  queryData: (q: GridQuery) => app().QueryData(q),
  getGridHeader: (datasetId: number) => app().GetGridHeader(datasetId),
  getArchive: () => app().GetArchive(),
  getYears: () => app().GetYears(),
  getWeeks: (year: number) => app().GetWeeks(year),
  getConfig: () => app().GetConfig(),
  saveConfig: (c: AppConfig) => app().SaveConfig(c),
  resetConfig: () => app().ResetConfig(),
  getLogs: (limit: number) => app().GetLogs(limit),
  clearLogs: () => app().ClearLogs(),
  quit: () => app().Quit(),
};

/** onEvent subscribes to a backend event and returns an unsubscribe function. */
export function onEvent<T>(name: string, cb: (payload: T) => void): () => void {
  const rt = window.runtime;
  if (!rt) return () => undefined;
  return rt.EventsOn(name, (...data: unknown[]) => cb(data[0] as T));
}
