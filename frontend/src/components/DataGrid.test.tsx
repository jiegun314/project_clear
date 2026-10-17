import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { DataGrid } from './DataGrid';
import type { GridHeader, GridResult, GridRow } from '../types';

vi.mock('../services/api', () => ({
  api: { getGridHeader: vi.fn(), queryData: vi.fn() },
}));

const { api } = await import('../services/api');
const mocked = vi.mocked(api);

const INDEX_NAMES = [
  'P5', 'P4', 'P3', 'P2', 'P1', 'MFG GROUP', 'MFG CLASS CODE', 'MFG DESCR',
  'ITEM', 'US CATALOG', 'EU CATALOG', 'LOC', 'BO', 'OH', 'LOC',
];

function header(datasetId: number, weekCode = ''): GridHeader {
  return {
    datasetId,
    kind: 'global',
    version: 1,
    weekCode,
    weekStart: weekCode ? '2026-09-21' : '',
    indexNames: INDEX_NAMES,
    weeks: weekCode ? [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }] : [],
    total: 0,
    expired: false,
  };
}

function row(seq: number, item: string): GridRow {
  return {
    seq,
    index: ['P5', 'P4', 'P3', 'P2', 'P1', 'GROUP A', 'CLASS 1', 'DESCR', item,
      'US-1', 'EU-1', 'WH_CNB', '0', '0', 'AdjDmd'],
    weeks: ['120'],
    weekMeta: [{}],
    fileName: 'synthetic.xlsm',
    srcRow: 57 + seq,
  };
}

const result = (rows: GridRow[], total = rows.length): GridResult => ({ rows, total, page: 1, pageSize: 200 });

function renderGrid(datasetID: number, reloadToken = 0) {
  return render(
    <DataGrid
      datasetID={datasetID}
      headerDisplay="twoRow"
      pageSize={200}
      reloadToken={reloadToken}
      title="数据清单"
      emptyText="这份数据还没有导入"
    />,
  );
}

describe('DataGrid', () => {
  it('renders the rows the backend returns and counts them', async () => {
    mocked.getGridHeader.mockResolvedValue(header(1, '2639'));
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001'), row(2, 'ITEM-0002')]));

    renderGrid(1);

    expect((await screen.findAllByText('ITEM-0001')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('ITEM-0002').length).toBeGreaterThan(0);
    expect(screen.getByText(/共 2 行/)).toBeTruthy();
    expect(screen.getByText('Global 数据 v1')).toBeTruthy();
    expect(mocked.getGridHeader).toHaveBeenCalledWith(1);
    expect(mocked.queryData).toHaveBeenCalledWith(expect.objectContaining({ datasetId: 1 }));
  });

  it('shows the empty state instead of stale rows when the query fails', async () => {
    mocked.getGridHeader.mockResolvedValue(header(1, '2639'));
    mocked.queryData.mockRejectedValue(new Error('backend unavailable'));

    renderGrid(1);

    expect(await screen.findByText('这份数据还没有导入')).toBeTruthy();
    expect(document.querySelectorAll('table')).toHaveLength(0);
  });

  it('keeps the newer result when an older request finishes later', async () => {
    let releaseFirst: (r: GridResult) => void = () => {};
    const first = new Promise<GridResult>((resolve) => {
      releaseFirst = resolve;
    });

    mocked.getGridHeader.mockResolvedValue(header(1, '2639'));
    mocked.queryData.mockReturnValueOnce(first);
    mocked.queryData.mockResolvedValue(result([row(2, 'NEWER')], 1));

    const view = renderGrid(1);
    await vi.waitFor(() => expect(mocked.queryData).toHaveBeenCalledTimes(1));
    view.rerender(
      <DataGrid datasetID={2} headerDisplay="twoRow" pageSize={200} reloadToken={0} title="数据清单" emptyText="这份数据还没有导入" />,
    );

    expect((await screen.findAllByText('NEWER')).length).toBeGreaterThan(0);
    releaseFirst(result([row(1, 'OLDER')], 1));
    await new Promise((r) => setTimeout(r, 20));

    expect(screen.queryByText('OLDER')).toBeNull();
  });

  it('looks again — header included — when the parent bumps the reload token', async () => {
    mocked.getGridHeader.mockResolvedValue(header(1, '2639'));
    mocked.queryData.mockResolvedValue(result([]));

    const view = renderGrid(1);
    await waitFor(() => expect(mocked.getGridHeader).toHaveBeenCalledTimes(1));

    mocked.getGridHeader.mockResolvedValue(header(1, '2639'));
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001')], 1516));
    view.rerender(
      <DataGrid datasetID={1} headerDisplay="twoRow" pageSize={200} reloadToken={1} title="数据清单" emptyText="这份数据还没有导入" />,
    );

    expect((await screen.findAllByText('ITEM-0001')).length).toBeGreaterThan(0);
    expect(mocked.getGridHeader).toHaveBeenCalledTimes(2);
    expect(screen.getByText(/共 1,516 行/)).toBeTruthy();
  });

  it('returns to the first page when the page size changes', async () => {
    mocked.getGridHeader.mockResolvedValue(header(1, '2639'));
    mocked.queryData.mockResolvedValue({ ...result([row(1, 'ITEM-0001')]), total: 900 });

    renderGrid(1);
    await screen.findAllByText('ITEM-0001');

    fireEvent.click(await screen.findByTitle('2'));
    await waitFor(() =>
      expect(mocked.queryData).toHaveBeenLastCalledWith(expect.objectContaining({ datasetId: 1, page: 2, pageSize: 200 })),
    );
    fireEvent.mouseDown(screen.getByRole('combobox'));
    fireEvent.click(await screen.findByText('500 / page'));
    await waitFor(() =>
      expect(mocked.queryData).toHaveBeenLastCalledWith(expect.objectContaining({ datasetId: 1, page: 1, pageSize: 500 })),
    );
  });
});
