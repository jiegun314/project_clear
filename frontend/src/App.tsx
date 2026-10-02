import { useCallback, useEffect, useRef, useState } from 'react';
import { App as AntApp, Modal, Tag, Tooltip, Spin, Alert } from 'antd';
import { Toolbar } from './components/Toolbar';
import { DataGrid } from './components/DataGrid';
import { HistoryPanel } from './components/HistoryPanel';
import { LogPanel } from './components/LogPanel';
import { SettingsDialog } from './components/SettingsDialog';
import { AboutDialog } from './components/AboutDialog';
import { ImportResultModal } from './components/ImportResultModal';
import { HistoryView } from './components/HistoryView';
import { StagedFilesModal } from './components/StagedFilesModal';
import { JNJ } from './theme/jnj';
import { api, hasBackend, onEvent } from './services/api';
import { SPLIT_BAR, TOP_MIN_HEIGHT, clampLower, lowerAfterDrag, splitBounds } from './lib/split';
import type {
  AppConfig,
  CommitResult,
  ExportResult,
  ImportResult,
  StagingFilesView,
  Status,
} from './types';

interface Progress {
  stage: string;
  done: number;
  total: number;
}

export default function App() {
  const [cfg, setCfg] = useState<AppConfig>({
    configVersion: 2,
    readColumns: 20,
    locFilter: 'WH_CNB',
    pageSize: 200,
    headerDisplay: 'twoRow',
    exportMode: 'clean',
    exportDir: '',
  });
  const [status, setStatus] = useState<Status | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<Progress | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  // The files of the current 整合清单, behind the toolbar's 已导入文件 button.
  const [stagedFiles, setStagedFiles] = useState<StagingFilesView | null>(null);
  const [stagedOpen, setStagedOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [historyWeek, setHistoryWeek] = useState<string | null>(null);
  // The main grid follows the working set: the staging area right after an
  // import, and the committed week once the data has been integrated. Without
  // this the grid would empty out at the moment the user integrates.
  const [gridSource, setGridSource] = useState('');
  const [notice, setNotice] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);
  // The two halves are resizable: the lower one carries an explicit height once
  // the bar has been dragged, clamped to 80%..120% of the height it has at
  // start-up. Null means "still the start-up flex split".
  const splitRef = useRef<HTMLDivElement>(null);
  const lowerRef = useRef<HTMLDivElement>(null);
  const [lowerHeight, setLowerHeight] = useState<number | null>(null);
  const [bounds, setBounds] = useState<{ min: number; max: number; start: number } | null>(null);
  const [dragging, setDragging] = useState(false);

  // Measured once, from the height the lower half has at start-up: the limits
  // follow the window the program opened with, not whatever size it is dragged
  // to afterwards. The observer stays until the first real measurement lands,
  // which is not guaranteed on the very first paint.
  useEffect(() => {
    if (bounds) return;
    const el = lowerRef.current;
    if (!el) return;
    const measure = () => {
      const start = el.offsetHeight;
      if (start <= 0) return;
      setBounds({ ...splitBounds(start), start });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [bounds]);

  // Locked to the height it started with: growing the window hands the extra
  // room to the data grid, and the history and log panels keep their size.
  // Only a drag (or a window too short to hold both) changes it.
  const lowerPx = lowerHeight ?? bounds?.start ?? null;

  // Keep the split honest when the window shrinks under it.
  useEffect(() => {
    if (!bounds) return;
    const fit = () => {
      const container = splitRef.current?.clientHeight ?? 0;
      if (container > 0) setLowerHeight((h) => clampLower(h ?? bounds.start, bounds, container));
    };
    window.addEventListener('resize', fit);
    return () => window.removeEventListener('resize', fit);
  }, [bounds]);

  const startResize = useCallback(
    (e: React.MouseEvent) => {
      const container = splitRef.current;
      const lower = lowerRef.current;
      if (!container || !lower || !bounds) return;
      e.preventDefault();
      // WebKit does not focus a non-form element on click, so the arrow keys
      // would have nothing to act on. Focusing the handle makes the keyboard
      // route work as well as the drag.
      (e.currentTarget as HTMLElement).focus();
      const from = e.clientY;
      const startedAt = lowerHeight ?? lower.offsetHeight;
      const containerHeight = container.clientHeight;
      setDragging(true);

      const move = (ev: MouseEvent) => {
        setLowerHeight(clampLower(lowerAfterDrag(startedAt, ev.clientY - from), bounds, containerHeight));
      };
      const stop = () => {
        window.removeEventListener('mousemove', move);
        window.removeEventListener('mouseup', stop);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        setDragging(false);
      };
      window.addEventListener('mousemove', move);
      window.addEventListener('mouseup', stop);
      document.body.style.cursor = 'row-resize';
      document.body.style.userSelect = 'none';
    },
    [bounds, lowerHeight],
  );

  const nudge = useCallback(
    (e: React.KeyboardEvent) => {
      if (!bounds) return;
      const container = splitRef.current?.clientHeight ?? 0;
      const step = e.key === 'ArrowUp' ? 16 : e.key === 'ArrowDown' ? -16 : 0;
      if (step === 0) return;
      e.preventDefault();
      const current = lowerHeight ?? lowerRef.current?.offsetHeight ?? bounds.min;
      setLowerHeight(clampLower(current + step, bounds, container));
    },
    [bounds, lowerHeight],
  );

  const refresh = useCallback(async () => {
    try {
      const [s, c, files] = await Promise.all([
        api.getStatus(),
        api.getConfig(),
        api.getStagingFiles(),
      ]);
      setStatus(s);
      setCfg(c.config);
      setStagedFiles(files);
    } catch {
      /* the shell may not be ready yet; the first poll will pick it up */
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const offProgress = onEvent<Progress>('task:progress', (p) => setProgress(p));
    const offDone = onEvent<{ ok: boolean; message?: string }>('task:done', (d) => {
      if (d && !d.ok && d.message) setNotice({ type: 'error', text: d.message });
    });
    return () => {
      offProgress();
      offDone();
    };
  }, []);

  const afterChange = async () => {
    setReloadToken((t) => t + 1);
    await refresh();
  };

  // ---- actions --------------------------------------------------------

  const finishImport = async (res: ImportResult) => {
    setImportResult(res);
    setImportOpen(true);
    setGridSource('');
    await afterChange();
  };

  const runImport = async (kind: 'import' | 'add') => {
    setBusy(true);
    setProgress({ stage: '准备中', done: 0, total: 0 });
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
            setBusy(true);
            setProgress({ stage: '重新读取', done: 0, total: 1 });
            try {
              const again = await api.confirmAddFiles(res.pendingPaths ?? []);
              if (again) await finishImport(again);
            } catch (e) {
              setNotice({ type: 'error', text: errorText(e) });
            } finally {
              setBusy(false);
              setProgress(null);
            }
          },
        });
        return;
      }
      await finishImport(res);
    } catch (e) {
      setNotice({ type: 'error', text: errorText(e) });
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  const runCommit = async () => {
    setBusy(true);
    setProgress({ stage: '整合入库', done: 0, total: 1 });
    try {
      const r: CommitResult = await api.commit();
      setGridSource(r.weekCode);
      setNotice({
        type: 'success',
        text: `周码 ${r.weekCode}（${r.weekStart}）已整合入库：${r.rowCount.toLocaleString()} 行 / ${r.fileCount} 个文件${r.overwrote ? '，已覆盖原有数据' : ''}`,
      });
      await afterChange();
    } catch (e) {
      setNotice({ type: 'error', text: errorText(e) });
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  const runExport = async () => {
    setBusy(true);
    setProgress({ stage: '导出', done: 0, total: 2 });
    try {
      // Empty mode: the toolbar follows whatever 参数设定 says (纯数据 by default).
      const r: ExportResult | null = await api.export(status?.weekCode ?? '', '');
      if (r) {
        setNotice({
          type: 'success',
          text: `已导出 ${r.rows.toLocaleString()} 行 × ${r.cols} 列（${r.mode === 'clean' ? '纯数据' : '原文件格式'}，条件格式 ${r.cfRows} 行、备注 ${r.comments} 条、宏${r.preservedVba ? '已保留' : '未包含'}）→ ${r.destPath}`,
        });
      }
    } catch (e) {
      setNotice({ type: 'error', text: errorText(e) });
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  // 清空 is destructive and cannot be undone, so it always asks first; the
  // dialog spells out how much is at stake and what is left alone.
  const runClear = () => {
    const staged =
      status?.hasStaging && status.stagedRows > 0
        ? `（周码 ${status.weekCode} · ${status.stagedRows.toLocaleString()} 行）`
        : '';
    Modal.confirm({
      title: '清空临时数据？',
      content: `将删除所有已导入但尚未整合入库的临时数据${staged}，已整合入库的数据不受影响。此操作不可撤销。`,
      okText: '清空',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setBusy(true);
        try {
          const r = await api.clearStaging();
          setGridSource('');
          setNotice({
            type: 'success',
            text:
              r && r.rows > 0
                ? `已清空临时数据：周码 ${r.weekCode}，共 ${r.rows.toLocaleString()} 行`
                : '已清空临时数据',
          });
          await afterChange();
        } catch (e) {
          setNotice({ type: 'error', text: errorText(e) });
        } finally {
          setBusy(false);
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

  if (!hasBackend()) {
    return (
      <div style={{ padding: 40 }}>
        <Alert
          type="error"
          showIcon
          message="未检测到 CLEAR 后端"
          description="请通过打包好的 CLEAR 桌面程序运行本界面，不要直接在浏览器中打开。"
        />
      </div>
    );
  }

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column', background: JNJ.bg }}>
      <Toolbar
        busy={busy}
        hasStaging={Boolean(status?.hasStaging)}
        hasArchive={(status?.archivedWeeks ?? 0) > 0}
        stagedFiles={stagedFiles}
        onImport={() => void runImport('import')}
        onAdd={() => void runImport('add')}
        onCommit={() => void runCommit()}
        onClear={runClear}
        onExport={() => void runExport()}
        onViewStagedFiles={() => setStagedOpen(true)}
        onSettings={() => setSettingsOpen(true)}
        onAbout={() => setAboutOpen(true)}
        onQuit={runQuit}
      />

      {notice && (
        <div style={{ padding: '6px 14px 0' }}>
          <Alert
            type={notice.type}
            closable
            showIcon
            message={notice.text}
            onClose={() => setNotice(null)}
            style={{ padding: '2px 8px' }}
          />
        </div>
      )}

      {/* Main content: merged data on top, history and logs beneath. */}
      <div
        ref={splitRef}
        style={{ flex: '1 1 auto', minHeight: 0, display: 'flex', flexDirection: 'column', padding: '8px 10px 0' }}
      >
        <div style={{ flex: '1 1 62%', minHeight: TOP_MIN_HEIGHT, display: 'flex', flexDirection: 'column' }}>
          <DataGrid
            source={gridSource}
            headerDisplay={cfg.headerDisplay}
            pageSize={cfg.pageSize}
            reloadToken={reloadToken}
          />
        </div>
        {/* Drag to resize, double click to go back to the start-up split. */}
        <div
          role="separator"
          aria-orientation="horizontal"
          aria-label="调整上下区域高度"
          aria-valuenow={Math.round(lowerPx ?? 0)}
          aria-valuemin={bounds?.min ?? 0}
          aria-valuemax={bounds?.max ?? 0}
          aria-valuetext={`下方区域 ${Math.round(lowerPx ?? 0)} 像素，可调范围 ${bounds?.min ?? 0}–${bounds?.max ?? 0}`}
          tabIndex={0}
          title="拖动调整高度，双击恢复默认"
          onMouseDown={startResize}
          onDoubleClick={() => setLowerHeight(null)}
          onKeyDown={nudge}
          style={{
            flex: `0 0 ${SPLIT_BAR}px`,
            cursor: 'row-resize',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: dragging ? JNJ.redSoft : 'transparent',
            outline: 'none',
          }}
        >
          <div
            style={{
              width: 44,
              height: 3,
              borderRadius: 2,
              background: dragging ? JNJ.red : JNJ.border,
            }}
          />
        </div>
        <div
          ref={lowerRef}
          style={{
            flex: lowerPx == null ? '1 1 38%' : `0 0 ${lowerPx}px`,
            minHeight: 0,
            display: 'flex',
            borderTop: `1px solid ${JNJ.border}`,
          }}
        >
          <HistoryPanel onView={(w) => setHistoryWeek(w)} reloadToken={reloadToken} />
          <div style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
            <LogPanel />
          </div>
        </div>
      </div>

      <StatusBar status={status} progress={progress} />

      <ImportResultModal open={importOpen} result={importResult} onClose={() => setImportOpen(false)} />
      <StagedFilesModal open={stagedOpen} view={stagedFiles} onClose={() => setStagedOpen(false)} />
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSaved={(c) => {
          setCfg(c);
          void afterChange();
        }}
      />
      <AboutDialog open={aboutOpen} onClose={() => setAboutOpen(false)} />
      <HistoryView
        weekCode={historyWeek}
        onClose={() => setHistoryWeek(null)}
        onExported={(m) => setNotice({ type: 'success', text: m })}
      />
    </div>
  );
}

function StatusBar({ status, progress }: { status: Status | null; progress: Progress | null }) {
  const items: { label: string; value: string; tip?: string }[] = [
    { label: '临时数据', value: status?.hasStaging ? `${status.weekCode} · ${status.stagedRows.toLocaleString()} 行` : '无' },
    {
      label: '已整合',
      value: `${status?.archivedWeeks ?? 0} 周 · ${(status?.archivedRows ?? 0).toLocaleString()} 行`,
    },
    { label: '读取列数', value: String(status?.readColumns ?? '-') },
    { label: 'LOC 筛选', value: status?.locFilter ?? '-' },
    { label: '每页行数', value: String(status?.pageSize ?? '-') },
    { label: '导出模式', value: status?.exportMode === 'clean' ? '纯数据' : '原文件格式' },
  ];
  return (
    <div
      style={{
        flex: '0 0 auto',
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        padding: '5px 14px',
        background: JNJ.surface,
        borderTop: `1px solid ${JNJ.border}`,
        fontSize: 12,
        color: JNJ.text,
      }}
    >
      {progress && (
        <span style={{ color: JNJ.red, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <Spin size="small" />
          {progress.stage}
          {progress.total > 1 ? ` ${progress.done}/${progress.total}` : ''}
        </span>
      )}
      {items.map((i) => (
        <Tooltip key={i.label} title={i.tip ?? `${i.label}：${i.value}`}>
          <span>
            <span style={{ color: JNJ.textMuted }}>{i.label} </span>
            <span style={{ fontWeight: 600, color: JNJ.ink }}>{i.value}</span>
          </span>
        </Tooltip>
      ))}
      {status?.database && (
        <Tooltip title={`数据库：${status.database}`}>
          <span style={{ marginLeft: 'auto', color: JNJ.textMuted, fontSize: 11 }}>本地数据库</span>
        </Tooltip>
      )}
    </div>
  );
}

function errorText(e: unknown): string {
  if (typeof e === 'string') return e;
  if (e instanceof Error) return e.message;
  return String(e);
}
