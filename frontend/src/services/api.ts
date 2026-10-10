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
  ClearResult,
  CommitResult,
  ConfigView,
  ExportResult,
  GridHeader,
  GridQuery,
  GridResult,
  ImportResult,
  LogEntry,
  StagingFilesView,
  Status,
} from '../types';

interface AppBindings {
  GetAppInfo(): Promise<AppInfo>;
  GetStatus(): Promise<Status>;
  ImportFolder(): Promise<ImportResult | null>;
  AddFiles(): Promise<ImportResult | null>;
  Commit(): Promise<CommitResult>;
  ClearStaging(): Promise<ClearResult>;
  Export(weekCode: string, mode: string): Promise<ExportResult | null>;
  RevealExport(path: string): Promise<void>;
  QueryData(q: GridQuery): Promise<GridResult>;
  GetGridHeader(source: string): Promise<GridHeader>;
  GetArchive(): Promise<ArchiveEntry[]>;
  GetYears(): Promise<number[]>;
  GetWeeks(year: number): Promise<number[]>;
  GetConfig(): Promise<ConfigView>;
  SaveConfig(c: AppConfig): Promise<string[]>;
  ResetConfig(): Promise<ConfigView>;
  GetLogs(limit: number): Promise<LogEntry[]>;
  GetStagingFiles(): Promise<StagingFilesView>;
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
  /** Adds files to the list; a file with the same name replaces its entry. */
  addFiles: () => app().AddFiles(),
  commit: () => app().Commit(),
  /** Drops every imported-but-unsaved row; integrated weeks are untouched. */
  clearStaging: () => app().ClearStaging(),
  /** mode is "", "clean" or "template"; "" follows the configured default. */
  export: (weekCode: string, mode: string) => app().Export(weekCode, mode),
  queryData: (q: GridQuery) => app().QueryData(q),
  getGridHeader: (source: string) => app().GetGridHeader(source),
  getArchive: () => app().GetArchive(),
  getYears: () => app().GetYears(),
  getWeeks: (year: number) => app().GetWeeks(year),
  getConfig: () => app().GetConfig(),
  saveConfig: (c: AppConfig) => app().SaveConfig(c),
  resetConfig: () => app().ResetConfig(),
  getLogs: (limit: number) => app().GetLogs(limit),
  getStagingFiles: () => app().GetStagingFiles(),
  clearLogs: () => app().ClearLogs(),
  quit: () => app().Quit(),
};

/** onEvent subscribes to a backend event and returns an unsubscribe function. */
export function onEvent<T>(name: string, cb: (payload: T) => void): () => void {
  const rt = window.runtime;
  if (!rt) return () => undefined;
  return rt.EventsOn(name, (...data: unknown[]) => cb(data[0] as T));
}

