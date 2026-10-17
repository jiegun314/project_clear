import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import App from './App';
import type {
  AppConfig,
  GridHeader,
  GridResult,
  ImportResult,
  StagingFilesView,
  Status,
} from './types';

// The shell's behaviour, pinned against the implementation as it stands so that
// the state it holds can be moved into hooks without anything changing for the
// user. These assertions are deliberately about what is on the screen — the
// status bar's values, which dialog opened, the error that was reported, where
// the split sits — and never about which hook holds what.
//
// Not covered here: the history loading overlay, because reaching it needs a
// week selected in the archive panel's dropdown. HistoryView's own tests cover
// the onReady signal the overlay waits for.

vi.mock('./services/api', () => ({
  api: {
    getStatus: vi.fn(),
    getConfig: vi.fn(),
    getStagingFiles: vi.fn(),
    importFolder: vi.fn(),
    importLocalMPS: vi.fn(),
    exportBundle: vi.fn(),
    addFiles: vi.fn(),
    confirmAddFiles: vi.fn(),
    commit: vi.fn(),
    clearStaging: vi.fn(),
    export: vi.fn(),
    quit: vi.fn(),
    getGridHeader: vi.fn(),
    queryData: vi.fn(),
    getArchive: vi.fn(),
    getYears: vi.fn(),
    getWeeks: vi.fn(),
    getLogs: vi.fn(),
    clearLogs: vi.fn(),
    getAppInfo: vi.fn(),
    saveConfig: vi.fn(),
    resetConfig: vi.fn(),
  },
  hasBackend: vi.fn(() => true),
  onEvent: vi.fn(() => () => {}),
}));

const { api, hasBackend } = await import('./services/api');
const mocked = vi.mocked(api);
const mockedHasBackend = vi.mocked(hasBackend);

const CONFIG: AppConfig = {
  configVersion: 2,
  readColumns: 20,
  locFilter: 'WH_CNB',
  pageSize: 200,
  headerDisplay: 'twoRow',
  exportMode: 'clean',
  exportDir: '',
};

const STATUS: Status = {
  hasStaging: true,
  weekCode: '2639',
  weekStart: '2026-09-21',
  stagedRows: 1736,
  archivedWeeks: 1,
  archivedRows: 1516,
  readColumns: 20,
  locFilter: 'WH_CNB',
  pageSize: 200,
  headerDisplay: 'twoRow',
  exportMode: 'clean',
  database: '/tmp/clear.db',
  lastAction: '',
  hasGlobal: true,
  hasLocal: true,
  globalRows: 1516,
  localRows: 1736,
  lastDataset: 'local',
};

const STAGED: StagingFilesView = {
  hasStaging: true,
  fileCount: 13,
  rowCount: 1516,
  failedCount: 0,
  batchState: 'staging',
  files: [],
};

const HEADER: GridHeader = {
  source: '',
  weekCode: '',
  weekStart: '',
  indexNames: [
    'P5', 'P4', 'P3', 'P2', 'P1', 'MFG GROUP', 'MFG CLASS CODE', 'MFG DESCR',
    'ITEM', 'US CATALOG', 'EU CATALOG', 'LOC', 'BO', 'OH', 'LOC',
  ],
  weeks: [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }],
  total: 0,
  hasStaging: true,
};

const EMPTY_RESULT: GridResult = { rows: [], total: 0, page: 1, pageSize: 200 };

const IMPORT_RESULT: ImportResult = {
  action: 'import',
  weekCode: '2639',
  weekStart: '2026-09-21',
  weekCodes: [],
  indexNames: [],
  total: 13,
  ok: 13,
  failed: 0,
  rowsKept: 1516,
  files: [],
  warnings: [],
  durationMs: 23714,
  localRows: 0,
};

const IMPORT_BUTTON = '导入 Global 文件夹：读取本周 MPS 导出的全部源文件';
const ADD_BUTTON = '添加 Global 源文件：把漏掉的文件补进当前清单';
const IMPORT_LOCAL_BUTTON = '导入本地 MPS 文件（Month End）：检查格式并写入 Local 数据';
const EXPORT_BUTTON = '导出：整合后生成 MPS / Local / Global 三表文件';
const COMMIT_BUTTON = '整合：合并 Global 与 Local，并写入永久周数据表';
const QUIT_BUTTON = '退出程序';

beforeEach(() => {
  // restoreMocks runs before every test, so each mock's implementation has to be
  // put back — including hasBackend, which decides whether anything renders.
  mockedHasBackend.mockReturnValue(true);
  mocked.getStatus.mockResolvedValue(STATUS);
  mocked.getConfig.mockResolvedValue({ config: CONFIG, path: '/tmp/config/clear.yaml' });
  mocked.getStagingFiles.mockResolvedValue(STAGED);
  mocked.getGridHeader.mockResolvedValue(HEADER);
  mocked.queryData.mockResolvedValue(EMPTY_RESULT);
  mocked.getArchive.mockResolvedValue([]);
  mocked.getYears.mockResolvedValue([2026]);
  mocked.getWeeks.mockResolvedValue([39]);
  mocked.getLogs.mockResolvedValue([]);
});

/** Waits until the first status refresh has landed in the status bar. */
async function renderApp(waitFor = '2639 · Global 1,516 / Local 1,736') {
  const view = render(<App />);
  await screen.findByText(waitFor);
  return view;
}

describe('App', () => {
  it('tells the user to run the desktop application when there is no backend', () => {
    mockedHasBackend.mockReturnValue(false);

    render(<App />);

    expect(screen.getByText('未检测到 CLEAR 后端')).toBeTruthy();
  });

  it('shows the backend status in the status bar', async () => {
    await renderApp();

    expect(screen.getByText('1 周 · 1,516 行')).toBeTruthy();
    expect(screen.getByText('WH_CNB')).toBeTruthy();
    expect(screen.getByText('纯数据')).toBeTruthy();
    expect(screen.getByText('本地数据库')).toBeTruthy();
  });

  it('opens the import summary after a successful import', async () => {
    mocked.importFolder.mockResolvedValue(IMPORT_RESULT);

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }));

    // The summary is the modal, not a notice: 合并行数 is one of its statistics.
    expect(await screen.findByText('导入行数')).toBeTruthy();
  });

  it('reports a failed import and does not open the summary', async () => {
    mocked.importFolder.mockRejectedValue(new Error('磁盘已满'));

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }));

    expect(await screen.findByText('磁盘已满')).toBeTruthy();
    expect(screen.queryByText('导入行数')).toBeNull();
  });

  it('only offers 整合 when both inputs are in the workbench', async () => {
    mocked.getStatus.mockResolvedValue({
      ...STATUS,
      hasStaging: true,
      hasGlobal: true,
      hasLocal: false,
      localRows: 0,
      lastDataset: 'global',
    });
    const { unmount } = render(<App />);
    const missingLocal = await screen.findByRole('button', { name: COMMIT_BUTTON });
    await waitFor(() => expect(missingLocal.hasAttribute('disabled')).toBe(true));
    unmount();

    mocked.getStatus.mockResolvedValue({ ...STATUS, hasStaging: false, hasGlobal: false, hasLocal: false, stagedRows: 0 });
    const empty = render(<App />);
    const noWorkbench = await screen.findByRole('button', { name: COMMIT_BUTTON });
    await waitFor(() => expect(noWorkbench.hasAttribute('disabled')).toBe(true));
    empty.unmount();

    mocked.getStatus.mockResolvedValue(STATUS);
    render(<App />);
    await waitFor(() =>
      expect(screen.getByRole('button', { name: COMMIT_BUTTON }).hasAttribute('disabled')).toBe(false),
    );
  });

  it('shows the committed week in the grid after 整合', async () => {
    // The grid renders the week from the header the backend returns for it.
    mocked.getGridHeader.mockImplementation(async (source: string) =>
      source === '2639' ? { ...HEADER, source, weekCode: '2639', weekStart: '2026-09-21' } : HEADER,
    );
    mocked.commit.mockResolvedValue({
      weekCode: '2639',
      weekStart: '2026-09-21',
      tableName: 'data_2639',
      rowCount: 2580,
      fileCount: 14,
      committedAt: '2026-10-05 21:16:46',
      overwrote: false,
      mergedRows: 2580,
      globalRows: 1516,
      localRows: 1736,
    });

    await renderApp();
    // While the staging area is the source, an empty grid says so.
    expect(await screen.findByText('这份数据还没有导入')).toBeTruthy();

    // The staged data is gone once it is integrated, which is what the strip and
    // the status bar read.
    mocked.getStatus.mockResolvedValue({ ...STATUS, hasStaging: false, stagedRows: 0 });
    fireEvent.click(screen.getByRole('button', { name: COMMIT_BUTTON }));

    // The report is a dialog with the numbers, not a line in the status bar.
    expect(await screen.findByText('周码 2639 已整合入库')).toBeTruthy();
    expect(screen.getByText('整合数据')).toBeTruthy();
    expect(screen.getByText('2,580 行')).toBeTruthy();
    expect(screen.getByText('1,516 行')).toBeTruthy();
    expect(screen.getByText('1,736 行')).toBeTruthy();
    expect(screen.getByText('本周此前没有数据')).toBeTruthy();
    // …and the week stays on screen, now labelled as integrated. The label 已整合
    // also appears in the status bar, so the strip is identified by its detail.
    expect(screen.getByText('周码 2639（2026-09-21）· 2,580 行 · 14 个文件')).toBeTruthy();
    // The list itself is named after that state, with the week it holds.
    expect(screen.getByText('数据清单 - 已整合（周码 2639）')).toBeTruthy();
    // The grid follows the committed week: it renders that week's header and is
    // no longer the staging area. 该周暂无数据 is deliberately not asserted on —
    // the archive panel renders the same sentence.
    expect(await screen.findByText('2639 · 2026-09-21')).toBeTruthy();
    await waitFor(() => expect(screen.queryByText('这份数据还没有导入')).toBeNull());
  });

  it('offers 导入本地 MPS and reports the workbench waiting for it', async () => {
    mocked.getStatus.mockResolvedValue({
      ...STATUS,
      hasGlobal: true,
      hasLocal: false,
      globalRows: 1516,
      localRows: 0,
      lastDataset: 'global',
      stagedRows: 1516,
    });
    mocked.importLocalMPS.mockResolvedValue({
      ...IMPORT_RESULT,
      action: 'local',
      ok: 1,
      total: 1,
      rowsKept: 1736,
      localRows: 1736,
    });

    await renderApp('2639 · Global 1,516');
    expect(screen.getByText('数据清单 - Global（待整合）')).toBeTruthy();
    expect(screen.getByText('待补齐')).toBeTruthy();
    expect(screen.getByText(/还需要导入本地 MPS 文件/)).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: IMPORT_LOCAL_BUTTON }));
    await waitFor(() => expect(mocked.importLocalMPS).toHaveBeenCalled());
    expect(await screen.findByText('本地 MPS 导入完成')).toBeTruthy();
    expect(screen.getByText('导入行数')).toBeTruthy();
  });

  it('accepts a local-first workbench and asks for Global before 整合', async () => {
    mocked.getStatus.mockResolvedValue({
      ...STATUS,
      hasGlobal: false,
      hasLocal: true,
      globalRows: 0,
      localRows: 1736,
      lastDataset: 'local',
      stagedRows: 1736,
    });

    await renderApp('2639 · Local 1,736');
    expect(screen.getByText('数据清单 - Local（待整合）')).toBeTruthy();
    expect(screen.getByText('待补齐')).toBeTruthy();
    expect(screen.getByText(/还需要导入 Global 数据/)).toBeTruthy();
    expect(screen.getByRole('button', { name: COMMIT_BUTTON }).hasAttribute('disabled')).toBe(true);
    expect(screen.getByRole('button', { name: IMPORT_LOCAL_BUTTON }).hasAttribute('disabled')).toBe(false);
  });

  it('exports the three-table file once the week is integrated', async () => {
    mocked.getStatus.mockResolvedValue({
      ...STATUS,
      hasStaging: false,
      weekCode: '',
      weekStart: '',
      stagedRows: 0,
      hasGlobal: true,
      hasLocal: true,
    });
    mocked.exportBundle.mockResolvedValue({
      destPath: '/tmp/BW_CN_MPS Week39 2026.xlsx',
      mode: 'bundle',
      rows: 2580,
      cols: 40,
      comments: 0,
      cfRows: 0,
      stylesUsed: 0,
      dropped: 0,
      sizeBytes: 9000,
      preservedVba: false,
      durationMs: 30,
      sheets: 3,
    });

    await renderApp('1 周 · 1,516 行');
    fireEvent.click(screen.getByRole('button', { name: EXPORT_BUTTON }));

    await waitFor(() => expect(mocked.exportBundle).toHaveBeenCalled());
    expect(mocked.export).not.toHaveBeenCalled();
    expect(await screen.findByText(/三表文件（MPS \/ Local \/ Global/)).toBeTruthy();
  });

  it('clears the previous notice when the next action begins', async () => {
    mocked.commit.mockRejectedValue(new Error('整合示例失败'));

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: COMMIT_BUTTON }));
    expect(await screen.findByText('整合示例失败')).toBeTruthy();

    mocked.importFolder.mockResolvedValue(IMPORT_RESULT);
    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }));
    // The stale failure must not sit above the grid while the new import runs.
    await waitFor(() => expect(screen.queryByText('整合示例失败')).toBeNull());
  });

  it('clears the previous notice when a window is opened', async () => {
    mocked.commit.mockRejectedValue(new Error('整合示例失败'));

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: COMMIT_BUTTON }));
    expect(await screen.findByText('整合示例失败')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: '查看已导入文件' }));
    await waitFor(() => expect(screen.queryByText('整合示例失败')).toBeNull());
  });

  it('resizes the lower half from the keyboard and restores it on double click', async () => {
    await renderApp();

    const handle = screen.getByRole('separator', { name: '调整上下区域高度' });
    // The lower half starts at 300px, measured from the stubbed layout.
    await waitFor(() => expect(handle.getAttribute('aria-valuenow')).toBe('300'));

    fireEvent.keyDown(handle, { key: 'ArrowUp' });
    await waitFor(() => expect(handle.getAttribute('aria-valuenow')).toBe('316'));

    fireEvent.doubleClick(handle);
    await waitFor(() => expect(handle.getAttribute('aria-valuenow')).toBe('300'));
  });

  it('asks before quitting, then asks the backend to quit', async () => {
    mocked.quit.mockResolvedValue(undefined);

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: QUIT_BUTTON }));

    // antd renders a two-character Chinese label with a space inside it
    // ("退 出"), so the name is matched loosely but anchored — that keeps the
    // toolbar's 退出程序 out of the match.
    fireEvent.click(await screen.findByRole('button', { name: /^退\s*出$/ }));
    await waitFor(() => expect(mocked.quit).toHaveBeenCalled());
  });
  it('names the list after what has happened to the data', async () => {
    // A fresh database: nothing staged, nothing integrated. The status the
    // refresh after the import returns is the ordinary one.
    mocked.getStatus.mockResolvedValueOnce({
      ...STATUS,
      hasStaging: false,
      stagedRows: 0,
      hasGlobal: false,
      hasLocal: false,
      globalRows: 0,
      localRows: 0,
      lastDataset: '',
    });
    mocked.getStatus.mockResolvedValue({
      ...STATUS,
      hasGlobal: true,
      hasLocal: false,
      localRows: 0,
      globalRows: 1516,
      lastDataset: 'global',
      stagedRows: 1516,
    });
    mocked.getStagingFiles.mockResolvedValue({ ...STAGED, hasStaging: false, fileCount: 0, rowCount: 0 });
    mocked.importFolder.mockResolvedValue(IMPORT_RESULT);

    render(<App />);
    // Nothing has been imported yet, so the list is just its own name.
    expect(await screen.findByText('数据清单')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }));

    expect(await screen.findByText('数据清单 - Global（待整合）')).toBeTruthy();
  });

  it('says which inputs are in the workbench and what is missing', async () => {
    await renderApp();

    // The strip above the table answers "which sides are ready".
    expect(screen.getByText('待整合')).toBeTruthy();
    expect(screen.getByText('周码 2639 · Global 1,516 行 · Local 1,736 行')).toBeTruthy();
    expect(screen.getByText(/两份数据已就绪/)).toBeTruthy();
  });

  it('switches the grid between the two imported sides', async () => {
    await renderApp();

    fireEvent.click(screen.getByRole('button', { name: '查看 Local 数据' }));
    await waitFor(() =>
      expect(mocked.queryData).toHaveBeenLastCalledWith(expect.objectContaining({ dataset: 'local' })),
    );

    fireEvent.click(screen.getByRole('button', { name: '查看 Global 数据' }));
    await waitFor(() =>
      expect(mocked.queryData).toHaveBeenLastCalledWith(expect.objectContaining({ dataset: 'global' })),
    );
  });

  it('replaces a same-named file without asking, and names what it replaced', async () => {
    mocked.addFiles.mockResolvedValue({
      ...IMPORT_RESULT,
      action: 'add',
      replaced: ['source.xlsm', 'extra.xlsm'],
    });

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: ADD_BUTTON }));

    // Straight to the summary: no overwrite prompt in between any more.
    expect(await screen.findByText('导入行数')).toBeTruthy();
    expect(screen.queryByText('该文件已经在整合清单里')).toBeNull();
    expect(screen.getByText(/已覆盖 2 个同名文件（source\.xlsm、extra\.xlsm）/)).toBeTruthy();
  });

  it('reports what 清空 discarded and leaves the area saying 已清空', async () => {
    mocked.clearStaging.mockResolvedValue({ weekCode: '2639', files: 13, rows: 1516 });

    await renderApp();
    // 清空 leaves no staging behind, which the refresh after it reads.
    mocked.getStatus.mockResolvedValue({ ...STATUS, hasStaging: false, stagedRows: 0 });
    fireEvent.click(screen.getByRole('button', { name: /^清空/ }));
    // 清空 is irreversible, so it still asks first.
    // The exact label with antd's inserted space: /清\s*空/ would also match the
    // toolbar's "清空：删除…" button and press that one again.
    fireEvent.click(await screen.findByRole('button', { name: '清 空' }));

    expect(await screen.findByText('已清空未整合的数据')).toBeTruthy();
    expect(screen.getByText('文件数')).toBeTruthy();
    expect(screen.getByText('13')).toBeTruthy();
    expect(screen.getByText('1,516')).toBeTruthy();
    expect(screen.getByText('已清空')).toBeTruthy();
    expect(screen.getByText('周码 2639 · 13 个文件 · 1,516 行')).toBeTruthy();
    expect(screen.getByText('数据清单 - 已清空')).toBeTruthy();
    expect(screen.getByText('已整合数据')).toBeTruthy();
  });
});
