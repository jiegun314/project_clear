import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import App from './App';
import type {
  AppConfig,
  CommitResult,
  ExportResult,
  FilesView,
  FinishResult,
  GridHeader,
  GridResult,
  ImportResult,
  Status,
} from './types';

vi.mock('./services/api', () => ({
  api: {
    getStatus: vi.fn(), getConfig: vi.fn(), getFiles: vi.fn(),
    importFolder: vi.fn(), addFiles: vi.fn(), finishGlobalMPS: vi.fn(), importLocalMPS: vi.fn(),
    commit: vi.fn(), discardDraft: vi.fn(), discardWeekImports: vi.fn(),
    exportDataset: vi.fn(), exportBundle: vi.fn(), quit: vi.fn(),
    getGridHeader: vi.fn(), queryData: vi.fn(),
    getArchive: vi.fn(), getYears: vi.fn(), getWeeks: vi.fn(),
    getLogs: vi.fn(), clearLogs: vi.fn(), getAppInfo: vi.fn(),
    saveConfig: vi.fn(), resetConfig: vi.fn(),
  },
  hasBackend: vi.fn(() => true),
  onEvent: vi.fn(() => () => {}),
}));

const { api, hasBackend } = await import('./services/api');
const mocked = vi.mocked(api);
const mockedHasBackend = vi.mocked(hasBackend);

const CONFIG: AppConfig = {
  configVersion: 2, readColumns: 20, locFilter: 'WH_CNB', pageSize: 200,
  headerDisplay: 'twoRow', exportMode: 'clean', exportDir: '',
};

const STATUS: Status = {
  hasDraft: false, draftId: 0, draftWeek: '', draftRows: 0, draftFiles: 0, draftFailed: 0,
  hasGlobal: true, globalId: 11, globalWeek: '2639', globalRows: 1516, globalFiles: 13,
  globalVersion: 2, globalAt: '2026-09-22 09:00:00',
  hasLocal: true, localId: 12, localWeek: '2639', localRows: 1736, localFiles: 1,
  localVersion: 1, localAt: '2026-09-22 10:00:00', localFileName: 'local.xlsx',
  hasIntegration: false, integrationId: 0, integrationWeek: '', integrationRows: 0,
  integrationAt: '', integrationExpired: false,
  activeWeek: '2639', unintegrated: true, canDiscardWeek: true,
  archivedWeeks: 0, archivedRows: 0,
  readColumns: 20, locFilter: 'WH_CNB', pageSize: 200, headerDisplay: 'twoRow',
  exportMode: 'clean', database: '/tmp/clear.db', lastAction: '',
};

const DRAFT: Status = {
  ...STATUS,
  hasDraft: true, draftId: 10, draftWeek: '2639', draftRows: 1516, draftFiles: 13,
  hasGlobal: false, globalId: 0, hasLocal: false, localId: 0,
  unintegrated: true, canDiscardWeek: true,
};

const INTEGRATED: Status = {
  ...STATUS,
  hasIntegration: true, integrationId: 100, integrationWeek: '2639', integrationRows: 2580,
  integrationAt: '2026-09-22 11:00:00', archivedWeeks: 1, archivedRows: 2580,
  unintegrated: false, canDiscardWeek: false,
};

const BLANK: Status = {
  ...STATUS,
  hasGlobal: false, globalId: 0, globalWeek: '', globalRows: 0, globalFiles: 0,
  globalVersion: 0, globalAt: '',
  hasLocal: false, localId: 0, localWeek: '', localRows: 0, localFiles: 0,
  localVersion: 0, localAt: '', localFileName: '',
  activeWeek: '', unintegrated: false, canDiscardWeek: false,
};

const FILES: FilesView = { files: [], hasDraft: false, fileCount: 0, rowCount: 0, failedCount: 0 };

const HEADER: GridHeader = {
  datasetId: 12, kind: 'local', version: 1, weekCode: '2639', weekStart: '2026-09-21',
  indexNames: ['P5', 'P4', 'P3', 'P2', 'P1', 'MFG GROUP', 'MFG CLASS CODE', 'MFG DESCR',
    'ITEM', 'US CATALOG', 'EU CATALOG', 'LOC', 'BO', 'OH', 'LOC'],
  weeks: [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }],
  total: 0, expired: false,
};

const EMPTY: GridResult = { rows: [], total: 0, page: 1, pageSize: 200 };

const IMPORT_RESULT: ImportResult = {
  action: 'import', weekCode: '2639', weekStart: '2026-09-21', weekCodes: [], indexNames: [],
  total: 13, ok: 13, failed: 0, rowsKept: 1516, files: [], warnings: [], durationMs: 23714,
  draft: true, datasetId: 10, localRows: 0, version: 0, expired: false, rowCount: 1516, fileCount: 13,
};

const FINISH_RESULT: FinishResult = {
  datasetId: 11, weekCode: '2639', weekStart: '2026-09-21', version: 2, rowCount: 1516,
  fileCount: 13, failedCount: 0, weekCodes: [], indexNames: [], importedAt: '2026-09-22 09:00:00',
  expired: false,
};

const COMMIT_RESULT: CommitResult = {
  weekCode: '2639', weekStart: '2026-09-21', mergedId: 100, globalId: 11, localId: 12,
  mergedRows: 2580, globalRows: 1516, localRows: 1736, globalVersion: 2, localVersion: 1,
  mergedVersion: 1, fileCount: 14, committedAt: '2026-09-22 11:00:00', overwrote: false,
  wasExpired: false,
};

const EXPORT_RESULT: ExportResult = {
  destPath: '/tmp/BW.xlsx', datasetId: 100, weekCode: '2639', mode: 'clean', rows: 2580,
  cols: 17, comments: 0, cfRows: 0, stylesUsed: 0, dropped: 0, sizeBytes: 1000,
  preservedVba: false, durationMs: 10, sheets: 3, expired: false,
};

const FINISH = '完成 Global 导入：把当前草稿定稿为可整合的数据集';
const COMMIT = '整合：合并已完成的 Global 与 Local，写入永久整合记录';
const EXPORT = '导出：生成 MPS / Local / Global 三表文件';
const CLEAR = '清空：丢弃未完成的 Global 导入';
const IMPORT = '导入 Global 文件夹：读取本周 MPS 导出的全部源文件（生成草稿）';
const LOCAL = '导入本地 MPS 文件（Month End）：检查格式并写入 Local 数据集';

beforeEach(() => {
  mockedHasBackend.mockReturnValue(true);
  mocked.getStatus.mockResolvedValue(STATUS);
  mocked.getConfig.mockResolvedValue({ config: CONFIG, path: '/tmp/clear.yaml' });
  mocked.getFiles.mockResolvedValue(FILES);
  mocked.getGridHeader.mockResolvedValue(HEADER);
  mocked.queryData.mockResolvedValue(EMPTY);
  mocked.getArchive.mockResolvedValue([]);
  mocked.getYears.mockResolvedValue([]);
  mocked.getWeeks.mockResolvedValue([]);
  mocked.getLogs.mockResolvedValue([]);
  mocked.importFolder.mockResolvedValue(IMPORT_RESULT);
  mocked.finishGlobalMPS.mockResolvedValue(FINISH_RESULT);
  mocked.commit.mockResolvedValue(COMMIT_RESULT);
  mocked.exportBundle.mockResolvedValue(EXPORT_RESULT);
  mocked.discardDraft.mockResolvedValue({ ...IMPORT_RESULT, action: 'discard', fileCount: 13, rowCount: 1516 });
  mocked.discardWeekImports.mockResolvedValue({ weekCode: '2639', datasets: 2 });
});

async function renderApp(waitForText = '2639 · Global v2 1,516 / Local v1 1,736') {
  const view = render(<App />);
  await screen.findByText(waitForText);
  return view;
}

describe('App', () => {
  it('tells the user to run the desktop application when there is no backend', () => {
    mockedHasBackend.mockReturnValue(false);
    render(<App />);
    expect(screen.getByText('未检测到 CLEAR 后端')).toBeTruthy();
  });

  it('shows the workspace counts in the status bar', async () => {
    await renderApp();
    expect(screen.getByText('2639 · Global v2 1,516 / Local v1 1,736')).toBeTruthy();
    expect(screen.getByText(/Local 文件：local\.xlsx/)).toBeTruthy();
    expect(screen.getByText('13 个文件 · 已导入')).toBeTruthy();
    expect(screen.getByText('Local 数据 v1')).toBeTruthy();
  });

  it('points the history panel at the current workbench week', async () => {
    mocked.getArchive.mockResolvedValue([
      { weekCode: '2639', weekStart: '2026-09-21', year: 2026, weekNo: 39, dataset: 'merged', datasetId: 100, version: 1, rowCount: 2580, fileCount: 14, committedAt: '2026-09-22 11:00:00', expired: false },
      { weekCode: '2640', weekStart: '2026-09-28', year: 2026, weekNo: 40, dataset: 'global', datasetId: 101, version: 1, rowCount: 1516, fileCount: 13, committedAt: '2026-10-18 09:00:00', expired: false },
    ]);
    mocked.getWeeks.mockResolvedValue([40, 39]);
    mocked.getStatus.mockResolvedValue({ ...STATUS, activeWeek: '2640' });
    await renderApp('2640 · Global v2 1,516 / Local v1 1,736');
    expect(await screen.findByText('第 40 周')).toBeTruthy();
  });

  it('starts with an empty workbench but keeps the history panel', async () => {
    mocked.getStatus.mockResolvedValue(BLANK);
    render(<App />);
    await screen.findByText('当前周');
    expect(await screen.findByText('暂无数据，请使用工具栏的导入或添加')).toBeTruthy();
    expect(mocked.getGridHeader).not.toHaveBeenCalled();
    expect(screen.queryByText('已导入文件')).toBeNull();
    expect(screen.queryByText('本周数据')).toBeNull();
  });

  it('does not call the Global file list expired when only the integration is', async () => {
    mocked.getStatus.mockResolvedValue({
      ...STATUS, hasIntegration: true, integrationId: 100, integrationWeek: '2639',
      integrationRows: 2580, integrationExpired: true, unintegrated: true,
    });
    await renderApp();
    expect(screen.getByText('13 个文件 · 已导入')).toBeTruthy();
    expect(screen.queryByText(/13 个文件 · 已过期/)).toBeNull();
  });

  it('opens the import summary and selects the new draft', async () => {
    mocked.getStatus.mockResolvedValueOnce(DRAFT).mockResolvedValue(DRAFT);
    await renderApp('2639 · Global 导入中 1,516');
    fireEvent.click(screen.getByRole('button', { name: IMPORT }));
    expect(await screen.findByText('已加入 Global 导入草稿')).toBeTruthy();
    expect(mocked.importFolder).toHaveBeenCalled();
  });

  it('reports a failed import instead of opening the summary', async () => {
    mocked.importFolder.mockRejectedValue(new Error('文件夹内没有找到 .xlsm 或 .xlsx 文件'));
    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: IMPORT }));
    expect(await screen.findByText('文件夹内没有找到 .xlsm 或 .xlsx 文件')).toBeTruthy();
    expect(screen.queryByText('已加入 Global 导入草稿')).toBeNull();
  });

  it('completes a draft and reports the new version', async () => {
    mocked.getStatus.mockResolvedValue(DRAFT);
    await renderApp('2639 · Global 导入中 1,516');
    fireEvent.click(screen.getByRole('button', { name: FINISH }));
    await waitFor(() => expect(mocked.finishGlobalMPS).toHaveBeenCalledWith(false));
    expect(await screen.findByText('周码 2639 的 Global 导入已完成')).toBeTruthy();
    expect(screen.getByText('Global v2')).toBeTruthy();
  });

  it('does not add a second notice bar after a Local import', async () => {
    mocked.importLocalMPS.mockResolvedValue({
      ...IMPORT_RESULT, action: 'local', draft: false, version: 4, expired: true,
      localRows: 1736, rowsKept: 1736, fileCount: 1, rowCount: 1736,
    });
    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: LOCAL }));
    expect(await screen.findByText('Local 导入完成')).toBeTruthy();
    expect(screen.queryByText(/Local 已导入（v4）/)).toBeNull();
  });

  it('asks for confirmation when files failed before completing the draft', async () => {
    mocked.getStatus.mockResolvedValue({ ...DRAFT, draftFailed: 2 });
    await renderApp('2639 · Global 导入中 1,516');
    fireEvent.click(screen.getByRole('button', { name: FINISH }));
    await waitFor(() => expect(screen.getAllByText('仍有文件读取失败').length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByRole('button', { name: '仍然完成' }).at(-1)!);
    await waitFor(() => expect(mocked.finishGlobalMPS).toHaveBeenCalledWith(true));
  });

  it('integrates completed inputs and reports the three counts', async () => {
    await renderApp();
    const commit = screen.getByRole('button', { name: COMMIT });
    expect(commit.hasAttribute('disabled')).toBe(false);
    fireEvent.click(commit);
    await waitFor(() => expect(mocked.commit).toHaveBeenCalled());
    expect(await screen.findByText('周码 2639 已整合入库')).toBeTruthy();
    expect(screen.getAllByText('2,580 行').length).toBeGreaterThan(0);
    expect(screen.getAllByText('v2 · 1,516 行').length).toBeGreaterThan(0);
    expect(screen.getAllByText('v1 · 1,736 行').length).toBeGreaterThan(0);
  });

  it('disables 整合 while a draft is open', async () => {
    mocked.getStatus.mockResolvedValue(DRAFT);
    await renderApp('2639 · Global 导入中 1,516');
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(true);
  });

  it('exports the three-table file only after integration', async () => {
    mocked.getStatus.mockResolvedValue(INTEGRATED);
    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: EXPORT }));
    await waitFor(() => expect(mocked.exportBundle).toHaveBeenCalledWith('2639'));
    expect(await screen.findByText('三表文件已导出')).toBeTruthy();
    expect(screen.getByText('/tmp/BW.xlsx')).toBeTruthy();
  });

  it('shows the dataset totals and the migration note in the file list', async () => {
    mocked.getFiles.mockResolvedValue({
      files: [], hasDraft: false, fileCount: 13, rowCount: 1516, failedCount: 0,
      note: '该周的数据由旧版本迁移而来，未保留逐文件清单；文件数、行数来自数据集记录。',
    });
    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: '查看 Global 已导入文件' }));
    expect(await screen.findByText('13 个文件 · 1,516 行')).toBeTruthy();
    expect(screen.getByText(/旧版本迁移/)).toBeTruthy();
  });

  it('discards the draft after confirmation', async () => {
    mocked.getStatus.mockResolvedValue(DRAFT);
    await renderApp('2639 · Global 导入中 1,516');
    fireEvent.click(screen.getByRole('button', { name: CLEAR }));
    await waitFor(() =>
      expect(screen.getAllByText('丢弃未完成的 Global 导入？').length).toBeGreaterThan(0),
    );
    fireEvent.click(screen.getAllByRole('button', { name: /丢\s*弃/ }).at(-1)!);
    await waitFor(() => expect(mocked.discardDraft).toHaveBeenCalled());
  });

  it('offers 丢弃本周导入 for un-integrated completed imports', async () => {
    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: '丢弃本周导入' }));
    await waitFor(() =>
      expect(screen.getAllByText('丢弃周码 2639 未整合的导入？').length).toBeGreaterThan(0),
    );
    fireEvent.click(screen.getAllByRole('button', { name: /丢\s*弃/ }).at(-1)!);
    await waitFor(() => expect(mocked.discardWeekImports).toHaveBeenCalledWith('2639'));
  });

  it('switches the grid between the completed sides', async () => {
    await renderApp();
    await waitFor(() => expect(mocked.getGridHeader).toHaveBeenCalled());
    mocked.getGridHeader.mockClear();
    fireEvent.click(screen.getByRole('button', { name: '查看 Global 数据' }));
    await waitFor(() => expect(mocked.getGridHeader).toHaveBeenCalledWith(11));
    mocked.getGridHeader.mockClear();
    fireEvent.click(screen.getByRole('button', { name: '查看 Local 数据' }));
    await waitFor(() => expect(mocked.getGridHeader).toHaveBeenCalledWith(12));
  });

  it('asks before quitting', async () => {
    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: '退出程序' }));
    await waitFor(() => expect(screen.getAllByText('退出 CLEAR？').length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByRole('button', { name: /退\s*出/ }).at(-1)!);
    await waitFor(() => expect(mocked.quit).toHaveBeenCalled());
  });
});
