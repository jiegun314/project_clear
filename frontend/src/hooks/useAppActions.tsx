import { useState } from 'react';
import { Modal } from 'antd';
import { api } from '../services/api';
import { errorText } from '../lib/errors';
import { JNJ } from '../theme/jnj';
import type { CommitResult, ExportResult, ImportResult } from '../types';
import type { BackendState } from './useBackendState';
import type { TaskRunner } from './useTaskRunner';

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


  /** 任务收尾：成功时留下绿色"完成"，失败或取消则清掉进度显示。 */
  const finishImport = async (res: ImportResult) => {
    setImportResult(res);
    setImportOpen(true);
    setGridSource('');
    await backend.afterChange();
  };

  const runImport = async (kind: 'import' | 'add') => {
    tasks.begin('准备中', 0, 0);
    let ok = false;
    try {
      const res = kind === 'import' ? await api.importFolder() : await api.addFiles();
      if (!res) return;
      if (res.needsConfirm) {
        // 添加 hit a workbook that is already in the list: ask before its rows
        // are replaced. Nothing has been written yet, so 取消 leaves the list
        // exactly as it was.
        const names = res.duplicateFiles ?? [];
        Modal.confirm({
          title: '该文件已经在整合清单里',
          content: (
            <div style={{ lineHeight: 1.7 }}>
              <div>{names.join('、')} 已经导入过，同一个文件不会重复添加。</div>
              <div style={{ color: JNJ.textMuted, fontSize: 12 }}>
                继续会用重新读取的数据覆盖它原有的数据，清单里的其他文件不受影响。
              </div>
            </div>
          ),
          okText: '覆盖原有数据',
          cancelText: '取消',
          onOk: async () => {
            tasks.begin('重新读取', 0, 1);
            let overwrote = false;
            try {
              const again = await api.confirmAddFiles(res.pendingPaths ?? []);
              if (again) {
                await finishImport(again);
                overwrote = true;
              }
            } catch (e) {
              tasks.setNotice({ type: 'error', text: errorText(e) });
            } finally {
              tasks.finish(overwrote);
            }
          },
        });
        return;
      }
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
      setGridSource(r.weekCode);
      tasks.setNotice({
        type: 'success',
        text: `周码 ${r.weekCode}（${r.weekStart}）已整合入库：${r.rowCount.toLocaleString()} 行 / ${r.fileCount} 个文件${r.overwrote ? '，已覆盖原有数据' : ''}`,
      });
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
          tasks.setNotice({
            type: 'success',
            text:
              r && r.rows > 0
                ? `已清空临时数据：周码 ${r.weekCode}，共 ${r.rows.toLocaleString()} 行`
                : recordOnly
                  ? '已清空导入文件列表'
                  : '已清空临时数据',
          });
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
