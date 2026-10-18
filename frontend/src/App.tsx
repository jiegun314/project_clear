import { Alert, Spin } from 'antd';
import { Toolbar } from './components/Toolbar';
import { DataGrid } from './components/DataGrid';
import { HistoryPanel } from './components/HistoryPanel';
import { LogPanel } from './components/LogPanel';
import { SettingsDialog } from './components/SettingsDialog';
import { AboutDialog } from './components/AboutDialog';
import { ImportResultModal } from './components/ImportResultModal';
import { HistoryView } from './components/HistoryView';
import { DataStatusStrip, emptyTextFor, titleFor } from './components/DataStatusStrip';
import { SummaryDialog } from './components/SummaryDialog';
import { StagedFilesModal } from './components/StagedFilesModal';
import { JNJ } from './theme/jnj';
import { hasBackend } from './services/api';
import { useBackendState } from './hooks/useBackendState';
import { useTaskRunner } from './hooks/useTaskRunner';
import { useAppActions } from './hooks/useAppActions';
import { SPLIT_BAR, TOP_MIN_HEIGHT } from './lib/split';
import { useSplitPane } from './hooks/useSplitPane';
import { StatusBar } from './components/StatusBar';

export default function App() {
  const backend = useBackendState();
  const tasks = useTaskRunner();
  // The dialog state and the toolbar actions live together: each action is what
  // opens the dialog it reports through.
  const actions = useAppActions(backend, tasks);
  const split = useSplitPane();

  const status = backend.status;
  const canIntegrate = Boolean(
    status?.hasGlobal &&
      status?.hasLocal &&
      !status?.hasDraft &&
      status?.globalWeek &&
      status?.globalWeek === status?.localWeek,
  );
  // This module belongs to Global only; Local is one workbook and is reported in
  // the status strip, so its count must not inflate the Global file count.
  const globalFiles = status?.hasDraft ? status.draftFiles : status?.globalFiles ?? 0;
  // The Global module describes the source workbooks only. Whether the week's
  // integration is current or expired belongs to the integration, not here.
  const countLabel = !status?.activeWeek
    ? '0 个文件'
    : status.hasDraft
      ? `${globalFiles} 个文件 · 导入中`
      : `${globalFiles} 个文件 · 已导入`;

  if (!hasBackend()) {
    return (
      <div style={{ padding: 40 }}>
        <Alert
          type="error"
          showIcon
          title="未检测到 CLEAR 后端"
          description="请通过打包好的 CLEAR 桌面程序运行本界面，不要直接在浏览器中打开。"
        />
      </div>
    );
  }

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column', background: JNJ.bg }}>
      <Toolbar
        busy={tasks.busy}
        hasDraft={Boolean(backend.status?.hasDraft)}
        hasGlobal={Boolean(backend.status?.hasGlobal)}
        hasLocal={Boolean(backend.status?.hasLocal)}
        canIntegrate={canIntegrate}
        hasIntegration={Boolean(backend.status?.hasIntegration)}
        integrationExpired={Boolean(backend.status?.integrationExpired)}
        countLabel={countLabel}
        failedCount={backend.files?.failedCount ?? backend.status?.draftFailed ?? 0}
        onImport={() => void actions.runImport('import')}
        onAdd={() => void actions.runImport('add')}
        onFinish={() => void actions.runFinishGlobal()}
        onImportLocal={() => void actions.runImportLocal()}
        onCommit={() => void actions.runCommit()}
        onClear={actions.runDiscardDraft}
        onExport={() => void actions.runToolbarExport()}
        onViewFiles={actions.openFiles}
        onSettings={actions.openSettings}
        onAbout={actions.openAbout}
        onQuit={actions.runQuit}
      />

      {tasks.notice && (
        <div style={{ padding: '6px 14px 0' }}>
          <Alert
            type={tasks.notice.type}
            closable
            showIcon
            title={tasks.notice.text}
            onClose={() => tasks.setNotice(null)}
            style={{ padding: '2px 8px' }}
          />
        </div>
      )}

      {/* Main content: merged data on top, history and logs beneath. */}
      <div
        ref={split.splitRef}
        style={{ flex: '1 1 auto', minHeight: 0, display: 'flex', flexDirection: 'column', padding: '8px 10px 0' }}
      >
        <div style={{ flex: '1 1 62%', minHeight: TOP_MIN_HEIGHT, display: 'flex', flexDirection: 'column' }}>
          {/* Where the data came from and whether it has been integrated: the
              table itself only answers "what are the numbers". */}
          <DataStatusStrip
            state={actions.dataState}
            onSelectDataset={actions.selectDataset}
            onDiscardWeek={actions.runDiscardWeek}
          />
          <DataGrid
            datasetID={actions.gridDatasetId}
            headerDisplay={backend.cfg.headerDisplay}
            pageSize={backend.cfg.pageSize}
            reloadToken={backend.reloadToken}
            title={titleFor(actions.dataState)}
            emptyText={emptyTextFor(actions.dataState)}
          />
        </div>
        {/* Drag to resize, double click to go back to the start-up split. */}
        <div
          role="separator"
          aria-orientation="horizontal"
          aria-label="调整上下区域高度"
          aria-valuenow={Math.round(split.lowerPx ?? 0)}
          aria-valuemin={split.limits?.min ?? 0}
          aria-valuemax={split.limits?.max ?? 0}
          aria-valuetext={`下方区域 ${Math.round(split.lowerPx ?? 0)} 像素，可调范围 ${split.limits?.min ?? 0}–${split.limits?.max ?? 0}`}
          tabIndex={0}
          title="拖动调整高度，双击恢复默认"
          onMouseDown={split.startResize}
          onDoubleClick={split.reset}
          onKeyDown={split.nudge}
          style={{
            flex: `0 0 ${SPLIT_BAR}px`,
            cursor: 'row-resize',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: split.dragging ? JNJ.redSoft : 'transparent',
            outline: 'none',
          }}
        >
          <div
            style={{
              width: 44,
              height: 3,
              borderRadius: 2,
              background: split.dragging ? JNJ.red : JNJ.border,
            }}
          />
        </div>
        <div
          ref={split.lowerRef}
          style={{
            flex: split.lowerPx == null ? '1 1 38%' : `0 0 ${split.lowerPx}px`,
            minHeight: 0,
            display: 'flex',
            borderTop: `1px solid ${JNJ.border}`,
          }}
        >
          <HistoryPanel
            onView={actions.openHistory}
            reloadToken={backend.reloadToken}
            activeWeek={backend.status?.activeWeek ?? ''}
          />
          <div style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
            <LogPanel />
          </div>
        </div>
      </div>

      <StatusBar status={backend.status} progress={tasks.progress} />

      <ImportResultModal open={actions.importOpen} result={actions.importResult} onClose={actions.closeImportSummary} />
        <SummaryDialog
          open={actions.summary.open}
          title={actions.summary.title}
          items={actions.summary.items}
          tone={actions.summary.tone}
          onClose={actions.closeSummary}
        />
      <StagedFilesModal open={actions.filesOpen} view={backend.files} onClose={actions.closeFiles} />
      <SettingsDialog
        open={actions.settingsOpen}
        onClose={actions.closeSettings}
        onSaved={(c) => {
          backend.applyConfig(c);
          void backend.afterChange();
        }}
      />
      <AboutDialog open={actions.aboutOpen} onClose={actions.closeAbout} />
      <HistoryView
        datasetId={actions.historyDatasetId}
        dataset={actions.historyDataset}
        onClose={actions.closeHistory}
        onExported={(m) => tasks.setNotice({ type: 'success', text: m })}
        onExport={(datasetId, mode) => actions.runExport(datasetId, mode)}
        onReady={actions.historyReady}
      />
      {actions.historyLoading && (
        <div
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 3000,
            background: 'rgba(255,255,255,0.72)',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 14,
          }}
        >
          <Spin size="large" />
          <span style={{ color: JNJ.text, fontSize: 13 }}>
            正在载入历史数据{actions.historyDatasetId ? '（数据集）' : ''}…
          </span>
        </div>
      )}
    </div>
  );
}
