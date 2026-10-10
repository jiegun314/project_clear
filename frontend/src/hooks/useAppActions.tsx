import { useState } from 'react';
import { Modal } from 'antd';
import { api } from '../services/api';
import { errorText } from '../lib/errors';
import type { SummaryItem } from '../components/SummaryDialog';
import type { CommitResult, ExportResult, ImportResult } from '../types';
import type { BackendState } from './useBackendState';
import type { TaskRunner } from './useTaskRunner';

/**
 * What the data area is holding right now — the three states the requirement
 * names, plus the neutral one before anything has happened.
 */
export type DataState =
  | { kind: 'none' }
  | { kind: 'imported'; weekCode: string; files: number; rows: number; replaced: string[] }
  | { kind: 'committed'; weekCode: string; weekStart: string; files: number; rows: number; overwrote: boolean }
  | { kind: 'cleared'; weekCode: string; files: number; rows: number };

/** Everything the toolbar, the panels and the shortcuts can ask the shell to do. */
export interface AppActions {
  // The dialogs an action reports through, and the state each one needs.
  importResult: ImportResult | null;
  importOpen: boolean;
  closeImportSummary: () => void;
  stagedOpen: boolean;
  openStagedFiles: () => void;
  closeStagedFiles: () => void;
  settingsOpen: boolean;
  openSettings: () => void;
  closeSettings: () => void;
  aboutOpen: boolean;
  openAbout: () => void;
  closeAbout: () => void;
  historyWeek: string | null;
  historyLoading: boolean;
  openHistory: (week: string) => void;
  closeHistory: () => void;
  /** The archive window has something on screen; the cover can come down. */
  historyReady: () => void;
  /** The main grid follows the working set: staging after an import, the committed week after 整合. */
  gridSource: string;
  /** 已导入（未整合）/ 已整合 / 已清空, shown above the table and in its empty state. */
  dataState: DataState;
  /** The dialog 整合入库 and 清空 report through. */
  summary: { open: boolean; title: string; tone: 'success' | 'neutral'; items: SummaryItem[] };
  closeSummary: () => void;
  // The actions themselves.
  runImport: (kind: 'import' | 'add') => Promise<void>;
  runCommit: () => Promise<void>;
  runExport: (weekCode?: string, mode?: string) => Promise<ExportResult | null>;
  runToolbarExport: () => Promise<void>;
  runClear: () => void;
  runQuit: () => void;
}

/**
 * What the shell does when the user clicks something: the five toolbar commands,
 * the confirmations they ask for, and the dialogs they report through.
 *
 * Kept apart from App so that the shell itself stays a description of the layout
 * — which is also where the rules that matter live: 清空 always asks first and
 * spells out what it will not touch, and every export goes through runExport so
 * the status bar has one source of progress.
 */
export function useAppActions(backend: BackendState, tasks: TaskRunner): AppActions {
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [stagedOpen, setStagedOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [historyWeek, setHistoryWeek] = useState<string | null>(null);
  // 历史数据量大，点开要等一会儿：点击那一刻就把全屏遮罩盖上，
  // 数据到位（或确认没有数据）后由 HistoryView 通知收起。
  const [historyLoading, setHistoryLoading] = useState(false);
  // The main grid follows the working set: the staging area right after an
  // import, and the committed week once the data has been integrated. Without
  // this the grid would empty out at the moment the user integrates.
  const [gridSource, setGridSource] = useState('');
  // The live staging (below) wins over this; it only answers "what did the last
  // 整合 or 清空 leave behind" once there is no staging left to ask about.
  const [lastEvent, setLastEvent] = useState<DataState>({ kind: 'none' });
  const [summary, setSummary] = useState<AppActions['summary']>({
    open: false,
    title: '',
    tone: 'success',
    items: [],
  });
  const showSummary = (title: string, items: SummaryItem[], tone: 'success' | 'neutral' = 'success') =>
    setSummary({ open: true, title, tone, items });


  /** 任务收尾：成功时留下绿色"完成"，失败或取消则清掉进度显示。 */
  const finishImport = async (res: ImportResult) => {
    setImportResult(res);
    setImportOpen(true);
    setGridSource('');
    setLastEvent({
      kind: 'imported',
      weekCode: res.weekCode,
      files: res.ok,
      rows: res.rowsKept,
      replaced: res.replaced ?? [],
    });
    await backend.afterChange();
  };

  const runImport = async (kind: 'import' | 'add') => {
    tasks.begin('准备中', 0, 0);
    let ok = false;
    try {
      // A file whose name is already in the list is replaced by this call — the
      // service does that without asking — so there is no confirmation branch
      // here. Which files were replaced comes back in the result and is reported
      // by the strip above the table and in the import summary.
      const res = kind === 'import' ? await api.importFolder() : await api.addFiles();
      if (!res) return;
      await finishImport(res);
      ok = true;
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  const runCommit = async () => {
    tasks.begin('整合入库', 0, 1);
    let ok = false;
    try {
      const r: CommitResult = await api.commit();
      // The week stays on screen after 整合 — it is the same data, now in the
      // archive — and the strip above the table says so.
      setGridSource(r.weekCode);
      setLastEvent({
        kind: 'committed',
        weekCode: r.weekCode,
        weekStart: r.weekStart,
        files: r.fileCount,
        rows: r.rowCount,
        overwrote: r.overwrote,
      });
      showSummary(`周码 ${r.weekCode} 已整合入库`, [
        { label: '周码', value: `${r.weekCode}（${r.weekStart}）`, strong: true },
        { label: '总行数', value: r.rowCount.toLocaleString() },
        { label: '文件数', value: String(r.fileCount) },
        { label: '原有数据', value: r.overwrote ? '已覆盖同一周的原有数据' : '本周此前没有数据' },
        { label: '整合时间', value: r.committedAt },
      ]);
      await backend.afterChange();
      ok = true;
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  /**
   * 导出：工具栏用当前临时/周数据的周码，历史数据窗口用它在看的那一周。
   * 两条路径都必须经过这里，状态栏的进度与完成标记才有一致的来源。
   */
  const runExport = async (weekCode?: string, mode = ''): Promise<ExportResult | null> => {
    tasks.begin('导出', 0, 2);
    let ok = false;
    try {
      // Empty mode: the toolbar follows whatever 参数设定 says (纯数据 by default).
      const r: ExportResult | null = await api.export(weekCode ?? backend.status?.weekCode ?? '', mode);
      ok = Boolean(r);
      return r;
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
      return null;
    } finally {
      tasks.finish(ok);
    }
  };

  /** 工具栏的导出：模式跟随参数设定，完成后给出完整摘要。 */
  const runToolbarExport = async () => {
    const r = await runExport();
    if (!r) return;
    tasks.setNotice({
      type: 'success',
      text: `已导出 ${r.rows.toLocaleString()} 行 × ${r.cols} 列（${r.mode === 'clean' ? '纯数据' : '原文件格式'}，条件格式 ${r.cfRows} 行、备注 ${r.comments} 条、宏${r.preservedVba ? '已保留' : '未包含'}）→ ${r.destPath}`,
    });
  };

  // 清空 is destructive and cannot be undone, so it always asks first; the
  // dialog spells out how much is at stake and what is left alone.
  const runClear = () => {
    const staged =
      backend.status?.hasStaging && backend.status.stagedRows > 0
        ? `（周码 ${backend.status.weekCode} · ${backend.status.stagedRows.toLocaleString()} 行）`
        : '';
    // After 整合 there is no temporary data left, but the 已导入文件 list keeps
    // showing that batch as its record; clearing it must not touch the week.
    const recordOnly = !backend.status?.hasStaging && (backend.stagedFiles?.fileCount ?? 0) > 0;
    Modal.confirm({
      title: recordOnly ? '清空已导入文件列表？' : '清空临时数据？',
      content: recordOnly
        ? '当前没有未整合的临时数据，将只清空「已导入文件」列表；已整合入库的数据不受影响。'
        : `将删除所有已导入但尚未整合入库的临时数据${staged}，已整合入库的数据不受影响。此操作不可撤销。`,
      okText: '清空',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        tasks.begin('清空', 0, 1);
        let ok = false;
        try {
          const r = await api.clearStaging();
          setGridSource('');
          setLastEvent({
            kind: 'cleared',
            weekCode: r?.weekCode ?? '',
            files: r?.files ?? 0,
            rows: r?.rows ?? 0,
          });
          showSummary(
            '已清空未整合的数据',
            r && r.rows > 0
              ? [
                  { label: '周码', value: r.weekCode, strong: true },
                  { label: '文件数', value: String(r.files) },
                  { label: '行数', value: r.rows.toLocaleString() },
                  { label: '已整合数据', value: '不受影响，仍在历史数据中' },
                ]
              : [
                  { label: '临时数据', value: '没有未整合的数据需要清空' },
                  { label: '已导入列表', value: recordOnly ? '已一并清空' : '本来就是空的' },
                  { label: '已整合数据', value: '不受影响，仍在历史数据中' },
                ],
            'neutral',
          );
          await backend.afterChange();
          ok = true;
        } catch (e) {
          tasks.setNotice({ type: 'error', text: errorText(e) });
        } finally {
          tasks.finish(ok);
        }
      },
    });
  };

  const runQuit = () => {
    Modal.confirm({
      title: '退出 CLEAR？',
      content: '已整合入库的数据会保留在本地数据库中。',
      okText: '退出',
      cancelText: '取消',
      onOk: () => api.quit(),
    });
  };

  return {
    // The live staging is the truth about "已导入，未整合"; without it, the last
    // 整合 or 清空 is what the data area is showing.
    dataState: backend.status?.hasStaging
      ? {
          kind: 'imported',
          weekCode: backend.status.weekCode,
          files: backend.stagedFiles?.fileCount ?? 0,
          rows: backend.status.stagedRows,
          replaced: lastEvent.kind === 'imported' ? lastEvent.replaced : [],
        }
      : lastEvent,
    summary,
    closeSummary: () => setSummary((v) => ({ ...v, open: false })),
    importResult,
    importOpen,
    closeImportSummary: () => setImportOpen(false),
    stagedOpen,
    openStagedFiles: () => setStagedOpen(true),
    closeStagedFiles: () => setStagedOpen(false),
    settingsOpen,
    openSettings: () => setSettingsOpen(true),
    closeSettings: () => setSettingsOpen(false),
    aboutOpen,
    openAbout: () => setAboutOpen(true),
    closeAbout: () => setAboutOpen(false),
    historyWeek,
    historyLoading,
    openHistory: (week: string) => {
      setHistoryLoading(true);
      setHistoryWeek(week);
    },
    closeHistory: () => {
      setHistoryWeek(null);
      setHistoryLoading(false);
    },
    historyReady: () => setHistoryLoading(false),
    gridSource,
    runImport,
    runCommit,
    runExport,
    runToolbarExport,
    runClear,
    runQuit,
  };
}
