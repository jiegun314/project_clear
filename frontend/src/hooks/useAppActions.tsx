import { useState } from 'react';
import { Modal } from 'antd';
import { api } from '../services/api';
import { errorText } from '../lib/errors';
import type { SummaryItem } from '../components/SummaryDialog';
import type { CommitResult, ExportResult, FinishResult, ImportResult, Status } from '../types';
import type { BackendState } from './useBackendState';
import type { TaskRunner } from './useTaskRunner';

/** Which dataset the grid is showing. */
export type SelectedKind = 'draft' | 'global' | 'local' | 'merged';

/**
 * What the data area is holding. The workspace is driven by the backend status:
 * the draft, the completed Global/Local datasets, and the week's integration
 * (which may be expired by a newer import).
 */
export type DataState =
  | { kind: 'none' }
  | {
      kind: 'workspace';
      status: Status;
      selected: number;
      selectedKind: SelectedKind;
      replaced: string[];
    }
  | { kind: 'cleared'; week: string; files: number; rows: number };

/** What the last draft/discard left behind, when the workspace is empty. */
type LastEvent =
  | { kind: 'none' }
  | { kind: 'workspace'; replaced: string[] }
  | { kind: 'cleared'; week: string; files: number; rows: number };

export interface AppActions {
  importResult: ImportResult | null;
  importOpen: boolean;
  closeImportSummary: () => void;
  filesOpen: boolean;
  openFiles: () => void;
  closeFiles: () => void;
  settingsOpen: boolean;
  openSettings: () => void;
  closeSettings: () => void;
  aboutOpen: boolean;
  openAbout: () => void;
  closeAbout: () => void;
  historyDatasetId: number | null;
  historyDataset: string;
  historyLoading: boolean;
  openHistory: (datasetId: number, dataset: string) => void;
  closeHistory: () => void;
  historyReady: () => void;
  /** Which dataset the main grid reads (0 when there is nothing). */
  gridDatasetId: number;
  /** Switches the grid to a dataset (draft/Global/Local/merged). */
  selectDataset: (datasetId: number) => void;
  dataState: DataState;
  summary: { open: boolean; title: string; tone: 'success' | 'neutral'; items: SummaryItem[] };
  closeSummary: () => void;
  runImport: (kind: 'import' | 'add') => Promise<void>;
  runFinishGlobal: () => Promise<void>;
  runImportLocal: () => Promise<void>;
  runCommit: () => Promise<void>;
  runExport: (datasetId?: number, mode?: string) => Promise<ExportResult | null>;
  runToolbarExport: () => Promise<void>;
  /** 清空: discard the open Global draft only. */
  runDiscardDraft: () => void;
  /** 丢弃本周导入: drop the week's un-integrated datasets; history untouched. */
  runDiscardWeek: () => void;
  runQuit: () => void;
}

function selectable(s: Status, id: number): boolean {
  return id > 0 && (id === s.draftId || id === s.globalId || id === s.localId || id === s.integrationId);
}

function kindOf(s: Status, id: number): SelectedKind {
  if (id !== 0 && id === s.draftId) return 'draft';
  if (id !== 0 && id === s.globalId) return 'global';
  if (id !== 0 && id === s.localId) return 'local';
  return 'merged';
}

/**
 * The default view follows the workflow: a current integration is what the
 * business hands on; otherwise the most recent completed import is shown. After
 * a new import makes the integration expired, the new import wins, so the user
 * sees what just arrived.
 */
function defaultSelection(s: Status): number {
  if (s.hasIntegration && !s.integrationExpired) return s.integrationId;
  if (s.hasDraft) return s.draftId;
  const g = s.hasGlobal ? Date.parse(s.globalAt) : -1;
  const l = s.hasLocal ? Date.parse(s.localAt) : -1;
  if (g >= 0 || l >= 0) return l > g ? s.localId : s.globalId;
  if (s.hasIntegration) return s.integrationId;
  return 0;
}

/**
 * The shell's actions: imports, the Global finish signal, Local import,
 * integration, exports, and the two discard paths.
 */
export function useAppActions(backend: BackendState, tasks: TaskRunner): AppActions {
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [filesOpen, setFilesOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [historyDatasetId, setHistoryDatasetId] = useState<number | null>(null);
  const [historyDataset, setHistoryDataset] = useState('merged');
  const [historyLoading, setHistoryLoading] = useState(false);
  // The dataset the grid should show. 0 means "follow the backend default".
  const [selectedDataset, setSelectedDataset] = useState(0);
  const [lastEvent, setLastEvent] = useState<LastEvent>({ kind: 'none' });
  const [summary, setSummary] = useState<AppActions['summary']>({
    open: false,
    title: '',
    tone: 'success',
    items: [],
  });
  const showSummary = (title: string, items: SummaryItem[], tone: 'success' | 'neutral' = 'success') =>
    setSummary({ open: true, title, tone, items });

  const openDialog = (setter: (open: boolean) => void) => {
    tasks.setNotice(null);
    setter(true);
  };

  const finishImport = async (res: ImportResult) => {
    setImportResult(res);
    setImportOpen(true);
    setSelectable(res.datasetId);
    setLastEvent({ kind: 'workspace', replaced: res.replaced ?? [] });
    await backend.afterChange();
  };

  const setSelectable = (id: number) => setSelectedDataset(id);

  const runImport = async (kind: 'import' | 'add') => {
    tasks.begin(kind === 'import' ? '导入 Global' : '添加 Global', 0, 0);
    let ok = false;
    try {
      const res = await (kind === 'import' ? api.importFolder() : api.addFiles());
      if (!res) return;
      await finishImport(res);
      ok = true;
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  const showFinishSummary = (r: FinishResult) => {
    showSummary(
      `周码 ${r.weekCode} 的 Global 导入已完成`,
      [
        { label: '版本', value: `Global v${r.version}`, strong: true },
        { label: '文件', value: `${r.fileCount} 个` },
        { label: '行数', value: r.rowCount.toLocaleString() },
        { label: '读取失败', value: r.failedCount > 0 ? `${r.failedCount} 个文件` : '无' },
        {
          label: '整合状态',
          value: r.expired
            ? `已有整合（${r.integrationAt}）已过期，请重新整合`
            : '已完成；两份数据齐备后即可整合',
        },
      ],
      r.expired ? 'neutral' : 'success',
    );
  };

  const finishNow = async (confirmFailures: boolean) => {
    tasks.begin('完成 Global 导入', 0, 2);
    let ok = false;
    try {
      const r = await api.finishGlobalMPS(confirmFailures);
      ok = true;
      setSelectedDataset(r.datasetId);
      setLastEvent({ kind: 'workspace', replaced: [] });
      await backend.afterChange();
      showFinishSummary(r);
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  const runFinishGlobal = async () => {
    const st = backend.status;
    if (!st?.hasDraft) return;
    tasks.setNotice(null);
    if (st.draftFailed > 0) {
      Modal.confirm({
        title: '仍有文件读取失败',
        content: `${st.draftFailed} 个文件没有读入。仍要完成这次 Global 导入吗？失败的文件会在「已导入文件」里保留原因。`,
        okText: '仍然完成',
        cancelText: '返回检查',
        onOk: () => finishNow(true),
      });
      return;
    }
    await finishNow(false);
  };

  const runImportLocal = async () => {
    tasks.begin('导入本地 MPS', 0, 0);
    let ok = false;
    try {
      const res = await api.importLocalMPS();
      if (!res) return;
      await finishImport(res);
      // No extra notice bar: the import result dialog and the one status strip
      // already say the integration is now expired.
      ok = true;
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  const runCommit = async () => {
    tasks.begin('整合', 0, 2);
    let ok = false;
    try {
      const r: CommitResult = await api.commit();
      ok = true;
      setSelectedDataset(r.mergedId);
      showSummary(`周码 ${r.weekCode} 已整合入库`, [
        { label: '周码', value: `${r.weekCode}（${r.weekStart}）`, strong: true },
        { label: '整合数据', value: `${r.mergedRows.toLocaleString()} 行`, strong: true },
        { label: 'Global', value: `v${r.globalVersion} · ${r.globalRows.toLocaleString()} 行` },
        { label: 'Local', value: `v${r.localVersion} · ${r.localRows.toLocaleString()} 行` },
        { label: '文件数', value: String(r.fileCount) },
        {
          label: '原有数据',
          value: r.wasExpired ? '已把过期的整合更新为最新版本' : r.overwrote ? '已覆盖原有整合' : '本周此前没有整合',
        },
        { label: '整合时间', value: r.committedAt },
      ]);
      await backend.afterChange();
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  const showExport = (r: ExportResult) => {
    const size =
      r.sizeBytes >= 1024 * 1024
        ? `${(r.sizeBytes / 1024 / 1024).toFixed(2)} MB`
        : `${Math.max(1, Math.round(r.sizeBytes / 1024))} KB`;
    const items: SummaryItem[] = [
      { label: '文件', value: r.destPath, strong: true },
      {
        label: '内容',
        value:
          r.sheets === 3
            ? 'MPS / Local / Global（3 个工作表）'
            : `${r.weekCode} · ${r.mode === 'clean' ? '纯数据' : '原文件格式'}`,
      },
      r.sheets === 3
        ? { label: '行数', value: `三表合计 ${r.rows.toLocaleString()} 行` }
        : { label: '行数', value: `${r.rows.toLocaleString()} 行 × ${r.cols} 列` },
      { label: '大小', value: size },
    ];
    if (r.expired) {
      items.push({ label: '提示', value: '该整合已过期，导出的是上次整合结果' });
    }
    showSummary(r.sheets === 3 ? '三表文件已导出' : '数据已导出', items, r.expired ? 'neutral' : 'success');
  };

  const runExport = async (datasetId?: number, mode = ''): Promise<ExportResult | null> => {
    if (!datasetId) return null;
    tasks.begin('导出', 0, 2);
    let ok = false;
    try {
      const r: ExportResult | null = await api.exportDataset(datasetId, mode);
      ok = Boolean(r);
      if (r) {
        showExport(r);
      }
      return r;
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
      return null;
    } finally {
      tasks.finish(ok);
    }
  };

  const runToolbarExport = async () => {
    const week = backend.status?.activeWeek ?? '';
    tasks.begin('导出三表', 0, 2);
    let ok = false;
    try {
      const r: ExportResult | null = await api.exportBundle(week);
      ok = Boolean(r);
      if (r) {
        showExport(r);
      }
    } catch (e) {
      tasks.setNotice({ type: 'error', text: errorText(e) });
    } finally {
      tasks.finish(ok);
    }
  };

  const runDiscardDraft = () => {
    const st = backend.status;
    if (!st?.hasDraft) return;
    tasks.setNotice(null);
    Modal.confirm({
      title: '丢弃未完成的 Global 导入？',
      content: `周码 ${st.draftWeek} · ${st.draftFiles} 个文件 · ${st.draftRows.toLocaleString()} 行。已完成的导入与整合记录不受影响。`,
      okText: '丢弃',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.discardDraft();
          setSelectedDataset(0);
          setLastEvent({ kind: 'cleared', week: st.draftWeek, files: st.draftFiles, rows: st.draftRows });
          await backend.afterChange();
          tasks.setNotice({ type: 'success', text: `已丢弃周码 ${st.draftWeek} 的 Global 导入草稿` });
        } catch (e) {
          tasks.setNotice({ type: 'error', text: errorText(e) });
        }
      },
    });
  };

  const runDiscardWeek = () => {
    const st = backend.status;
    if (!st?.activeWeek || !st.canDiscardWeek) return;
    tasks.setNotice(null);
    Modal.confirm({
      title: `丢弃周码 ${st.activeWeek} 未整合的导入？`,
      content:
        '会删除本周尚未整合的 Global / Local 数据；历史的整合记录不受影响。' +
        (st.hasDraft ? '当前未完成的 Global 草稿也会一并丢弃。' : ''),
      okText: '丢弃',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.discardWeekImports(st.activeWeek);
          setSelectedDataset(0);
          await backend.afterChange();
          tasks.setNotice({ type: 'success', text: `已丢弃周码 ${st.activeWeek} 未整合的导入` });
        } catch (e) {
          tasks.setNotice({ type: 'error', text: errorText(e) });
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

  const status = backend.status;
  let dataState: DataState = { kind: 'none' };
  if (status && status.activeWeek) {
    const selected = selectable(status, selectedDataset) ? selectedDataset : defaultSelection(status);
    dataState = {
      kind: 'workspace',
      status,
      selected,
      selectedKind: kindOf(status, selected),
      replaced: lastEvent.kind === 'workspace' ? lastEvent.replaced : [],
    };
  } else if (lastEvent.kind === 'cleared') {
    dataState = lastEvent;
  }

  return {
    dataState,
    summary,
    closeSummary: () => setSummary((v) => ({ ...v, open: false })),
    importResult,
    importOpen,
    closeImportSummary: () => setImportOpen(false),
    filesOpen,
    openFiles: () => openDialog(setFilesOpen),
    closeFiles: () => setFilesOpen(false),
    settingsOpen,
    openSettings: () => openDialog(setSettingsOpen),
    closeSettings: () => setSettingsOpen(false),
    aboutOpen,
    openAbout: () => openDialog(setAboutOpen),
    closeAbout: () => setAboutOpen(false),
    historyDatasetId,
    historyDataset,
    historyLoading,
    openHistory: (datasetId: number, dataset: string) => {
      tasks.setNotice(null);
      setHistoryDataset(dataset || 'merged');
      setHistoryLoading(true);
      setHistoryDatasetId(datasetId);
    },
    closeHistory: () => {
      setHistoryDatasetId(null);
      setHistoryLoading(false);
    },
    historyReady: () => setHistoryLoading(false),
    gridDatasetId: dataState.kind === 'workspace' ? dataState.selected : 0,
    selectDataset: (datasetId: number) => setSelectedDataset(datasetId),
    runImport,
    runFinishGlobal,
    runImportLocal,
    runCommit,
    runExport,
    runToolbarExport,
    runDiscardDraft,
    runDiscardWeek,
    runQuit,
  };
}
