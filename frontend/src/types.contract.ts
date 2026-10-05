/**
 * Compile-time check that the hand-written shapes still match the generated
 * bindings.
 *
 * `types.ts` mirrors what the Go side sends, and the generated
 * `wailsjs/go/models.ts` is the authoritative description of it. The two are
 * tied together here: `npm run typecheck` fails on this file when a Go field is
 * renamed, dropped or retyped, instead of the page reading `undefined` at
 * runtime - which is how the frontend and the backend drifted apart before.
 *
 * Two adjustments are needed to compare them:
 *
 *   - `Data<T>` drops the generator's helper members. Its classes are meant to
 *     be instantiated by the Wails runtime and carry a `convertValues` method,
 *     which is not part of the JSON the page receives.
 *   - Four shapes are narrowed by hand because the generator loses or widens
 *     something the page relies on, so those are checked in one direction:
 *     `Week.start` (time.Time arrives as `any`), the two `config.Config` value
 *     unions, `logging.Entry.level`, and `ConfigView.config`. `StagingFilesView`
 *     is the opposite case: an empty list omits `batchState`, so the frontend
 *     marks it optional and is the wider of the two.
 */
import type { config, logging, mps, service, store, view } from '../wailsjs/go/models';
import type {
  AppConfig,
  AppInfo,
  ArchiveEntry,
  CellMeta,
  ClearResult,
  CommitResult,
  ConfigView,
  ExportResult,
  FileResult,
  GridHeader,
  GridQuery,
  GridResult,
  GridRow,
  ImportResult,
  LogEntry,
  StagedFile,
  StagingFilesView,
  Status,
  Week,
} from './types';

/** Compiles only when the assertion holds; `false` is a type error here. */
type Assert<T extends true> = T;
/**
 * The data shape of a generated model: its `convertValues` helper is stripped,
 * recursively, because the models nest each other.
 */
type Data<T> = T extends (...args: never[]) => unknown
  ? never
  : T extends (infer U)[]
    ? Data<U>[]
    : T extends object
      ? { [K in keyof T as T[K] extends (...args: never[]) => unknown ? never : K]: Data<T[K]> }
      : T;
/** Compiles only when everything A can be is also a B. */
type Assignable<A, B> = [A] extends [B] ? true : false;

// The frontend's view must still fit the generated data shape. A field the Go
// side renames or drops leaves the generated type requiring something these
// types do not carry, so the assertion below fails and the drift is caught here
// rather than as an `undefined` in the page.
export type _AppInfo = Assert<Assignable<AppInfo, Data<view.AppInfo>>>;
export type _Status = Assert<Assignable<Status, Data<view.Status>>>;
export type _CommitResult = Assert<Assignable<CommitResult, Data<view.CommitResult>>>;
export type _ClearResult = Assert<Assignable<ClearResult, Data<view.ClearStagingResult>>>;
export type _ExportResult = Assert<Assignable<ExportResult, Data<view.ExportResult>>>;
export type _GridQuery = Assert<Assignable<GridQuery, Data<view.GridQuery>>>;
export type _GridHeader = Assert<Assignable<GridHeader, Data<view.GridHeader>>>;
export type _GridRow = Assert<Assignable<GridRow, Data<store.GridRow>>>;
export type _CellMeta = Assert<Assignable<CellMeta, Data<store.CellMeta>>>;
export type _GridResult = Assert<Assignable<GridResult, Data<store.GridResult>>>;
export type _ArchiveEntry = Assert<Assignable<ArchiveEntry, Data<store.ArchiveEntry>>>;
export type _StagedFile = Assert<Assignable<StagedFile, Data<store.StagedFileDetail>>>;
export type _ImportResult = Assert<Assignable<ImportResult, Data<service.ImportResult>>>;
export type _FileResult = Assert<Assignable<FileResult, Data<service.FileResult>>>;

// Narrowed by hand: what the frontend holds must still fit the generated shape.
export type _ConfigView = Assert<Assignable<ConfigView, Data<view.ConfigView>>>;
export type _LogEntry = Assert<Assignable<LogEntry, Data<logging.Entry>>>;
export type _Week = Assert<Assignable<Week, Data<mps.Week>>>;
export type _AppConfig = Assert<Assignable<AppConfig, Data<config.Config>>>;

// Deliberately wider than the generated shape (an omitted field is optional).
export type _StagingFilesView = Assert<Assignable<Data<view.StagingFilesView>, StagingFilesView>>;
