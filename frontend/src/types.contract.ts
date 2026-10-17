/**
 * Compile-time check that the hand-written shapes still match the generated
 * bindings. `types.ts` mirrors what the Go side sends, and the generated
 * `wailsjs/go/models.ts` is the authoritative description of it.
 */
import type { config, logging, mps, service, store, view } from '../wailsjs/go/models';
import type {
  AppConfig,
  AppInfo,
  ArchiveEntry,
  CellMeta,
  CommitResult,
  ConfigView,
  DiscardWeekResult,
  ExportResult,
  FileResult,
  FileWithDataset,
  FilesView,
  FinishResult,
  GridHeader,
  GridQuery,
  GridResult,
  GridRow,
  ImportResult,
  LogEntry,
  Status,
  Week,
} from './types';

type Assert<T extends true> = T;
type Data<T> = T extends (...args: never[]) => unknown
  ? never
  : T extends (infer U)[]
    ? Data<U>[]
    : T extends object
      ? { [K in keyof T as T[K] extends (...args: never[]) => unknown ? never : K]: Data<T[K]> }
      : T;
type Assignable<A, B> = [A] extends [B] ? true : false;

export type _AppInfo = Assert<Assignable<AppInfo, Data<view.AppInfo>>>;
export type _Status = Assert<Assignable<Status, Data<view.Status>>>;
export type _CommitResult = Assert<Assignable<CommitResult, Data<view.CommitResult>>>;
export type _ExportResult = Assert<Assignable<ExportResult, Data<view.ExportResult>>>;
export type _GridQuery = Assert<Assignable<GridQuery, Data<view.GridQuery>>>;
export type _GridHeader = Assert<Assignable<GridHeader, Data<view.GridHeader>>>;
export type _GridRow = Assert<Assignable<GridRow, Data<store.GridRow>>>;
export type _CellMeta = Assert<Assignable<CellMeta, Data<store.CellMeta>>>;
export type _GridResult = Assert<Assignable<GridResult, Data<store.GridResult>>>;
export type _ArchiveEntry = Assert<Assignable<ArchiveEntry, Data<service.ArchiveEntry>>>;
export type _ImportResult = Assert<Assignable<ImportResult, Data<service.ImportResult>>>;
export type _FinishResult = Assert<Assignable<FinishResult, Data<service.FinishResult>>>;
export type _FileResult = Assert<Assignable<FileResult, Data<service.FileResult>>>;
export type _FileWithDataset = Assert<Assignable<FileWithDataset, Data<service.FileWithDataset>>>;
export type _FilesView = Assert<Assignable<FilesView, Data<view.FilesView>>>;
export type _DiscardWeekResult = Assert<Assignable<DiscardWeekResult, Data<view.DiscardWeekResult>>>;
export type _ConfigView = Assert<Assignable<ConfigView, Data<view.ConfigView>>>;
export type _LogEntry = Assert<Assignable<LogEntry, Data<logging.Entry>>>;
export type _Week = Assert<Assignable<Week, Data<mps.Week>>>;
export type _AppConfig = Assert<Assignable<AppConfig, Data<config.Config>>>;
