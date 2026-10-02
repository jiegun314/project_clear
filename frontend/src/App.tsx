import { useCallback, useEffect, useState } from 'react';
import { App as AntApp, Modal, Tag, Tooltip, Spin, Alert } from 'antd';
import { Toolbar } from './components/Toolbar';
import { DataGrid } from './components/DataGrid';
import { HistoryPanel } from './components/HistoryPanel';
import { LogPanel } from './components/LogPanel';
import { SettingsDialog } from './components/SettingsDialog';
import { AboutDialog } from './components/AboutDialog';
import { ImportResultModal } from './components/ImportResultModal';
import { HistoryView } from './components/HistoryView';
import { JNJ } from './theme/jnj';
import { api, hasBackend, onEvent } from './services/api';
import type { AppConfig, CommitResult, ExportResult, ImportResult, Status } from './types';

interface Progress {
  stage: string;
  done: number;
  total: number;
}

export default function App() {
  const [cfg, setCfg] = useState<AppConfig>({
    readColumns: 20,
    locFilter: 'WH_CNB',
    pageSize: 200,
    headerDisplay: 'twoRow',
    exportMode: 'template',
    exportDir: '',
  });
  const [status, setStatus] = useState<Status | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<Progress | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [historyWeek, setHistoryWeek] = useState<string | null>(null);
  const [notice, setNotice] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [s, c] = await Promise.all([api.getStatus(), api.getConfig()]);
      setStatus(s);
      setCfg(c.config);
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

  const runImport = async (kind: 'import' | 'add') => {
    setBusy(true);
    setProgress({ stage: '准备中', done: 0, total: 0 });
    try {
      const res = kind === 'import' ? await api.importFolder() : await api.addFiles();
      if (res) {
        setImportResult(res);
        setImportOpen(true);
        await afterChange();
      }
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
      const r: ExportResult | null = await api.export(status?.weekCode ?? '', false);
      if (r) {
        setNotice({
          type: 'success',
          text: `已导出 ${r.rows.toLocaleString()} 行 × ${r.cols} 列（条件格式 ${r.cfRows} 行、注释 ${r.comments} 条、宏${r.preservedVba ? '已保留' : '未包含'}）→ ${r.destPath}`,
        });
      }
    } catch (e) {
      setNotice({ type: 'error', text: errorText(e) });
    } finally {
      setBusy(false);
      setProgress(null);
    }
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
        onImport={() => void runImport('import')}
        onAdd={() => void runImport('add')}
        onCommit={() => void runCommit()}
        onExport={() => void runExport()}
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
      <div style={{ flex: '1 1 auto', minHeight: 0, display: 'flex', flexDirection: 'column', padding: '8px 10px 0' }}>
        <div style={{ flex: '1 1 62%', minHeight: 200, display: 'flex', flexDirection: 'column' }}>
          <DataGrid
            source=""
            headerDisplay={cfg.headerDisplay}
            pageSize={cfg.pageSize}
            reloadToken={reloadToken}
          />
        </div>
        <div style={{ flex: '1 1 38%', minHeight: 160, display: 'flex', marginTop: 8, borderTop: `1px solid ${JNJ.border}` }}>
          <HistoryPanel onView={(w) => setHistoryWeek(w)} reloadToken={reloadToken} />
          <div style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
            <LogPanel />
          </div>
        </div>
      </div>

      <StatusBar status={status} progress={progress} />

      <ImportResultModal open={importOpen} result={importResult} onClose={() => setImportOpen(false)} />
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
    { label: '导出模式', value: status?.exportMode === 'clean' ? '干净重建' : '模板改写' },
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
