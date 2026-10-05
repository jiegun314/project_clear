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
  stagedRows: 1516,
  archivedWeeks: 1,
  archivedRows: 1516,
  readColumns: 20,
  locFilter: 'WH_CNB',
  pageSize: 200,
  headerDisplay: 'twoRow',
  exportMode: 'clean',
  database: '/tmp/clear.db',
  lastAction: '',
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
};

const IMPORT_BUTTON = '导入文件夹中的所有源文件';
const COMMIT_BUTTON = '整合：将临时数据写入永久周数据表';
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
async function renderApp() {
  const view = render(<App />);
  await screen.findByText('2639 · 1,516 行');
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
    expect(await screen.findByText('合并行数')).toBeTruthy();
  });

  it('reports a failed import and does not open the summary', async () => {
    mocked.importFolder.mockRejectedValue(new Error('磁盘已满'));

    await renderApp();
    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON }));

    expect(await screen.findByText('磁盘已满')).toBeTruthy();
    expect(screen.queryByText('合并行数')).toBeNull();
  });

  it('only offers 整合 when there is temporary data', async () => {
    mocked.getStatus.mockResolvedValue({ ...STATUS, hasStaging: false, weekCode: '', stagedRows: 0 });

    render(<App />);

    const commit = await screen.findByRole('button', { name: COMMIT_BUTTON });
    await waitFor(() => expect(commit.hasAttribute('disabled')).toBe(true));
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
      rowCount: 1516,
      fileCount: 13,
      committedAt: '2026-10-05 21:16:46',
      overwrote: false,
    });

    await renderApp();
    // While the staging area is the source, an empty grid says so.
    expect(await screen.findByText('暂无临时数据，请使用工具栏的导入或添加')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: COMMIT_BUTTON }));

    expect(await screen.findByText(/已整合入库/)).toBeTruthy();
    // The grid follows the committed week: it renders that week's header and is
    // no longer the staging area. 该周暂无数据 is deliberately not asserted on —
    // the archive panel renders the same sentence.
    expect(await screen.findByText('2639 · 2026-09-21')).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByText('暂无临时数据，请使用工具栏的导入或添加')).toBeNull(),
    );
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
});
