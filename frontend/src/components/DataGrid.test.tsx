import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { DataGrid } from './DataGrid';
import type { GridHeader, GridResult, GridRow } from '../types';

// The grid is the component most likely to be restructured — its fetch, paging,
// sorting and sizing are duplicated in HistoryView and are meant to be merged
// into one kernel. These tests pin the behaviour a reader of the screen can see,
// so that work can be done without guessing whether it changed anything. They
// deliberately do not assert on state or on how many times something was called
// internally: that would make the refactor harder, not safer.

vi.mock('../services/api', () => ({
  api: { getGridHeader: vi.fn(), queryData: vi.fn() },
}));

const { api } = await import('../services/api');
const mocked = vi.mocked(api);

const INDEX_NAMES = [
  'P5', 'P4', 'P3', 'P2', 'P1', 'MFG GROUP', 'MFG CLASS CODE', 'MFG DESCR',
  'ITEM', 'US CATALOG', 'EU CATALOG', 'LOC', 'BO', 'OH', 'LOC',
];

function header(source: string, weekCode = ''): GridHeader {
  return {
    source,
    weekCode,
    weekStart: weekCode ? '2026-09-21' : '',
    indexNames: INDEX_NAMES,
    weeks: [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }],
    total: 0,
    hasStaging: false,
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

function result(rows: GridRow[], total = rows.length): GridResult {
  return { rows, total, page: 1, pageSize: 200 };
}

function renderGrid(source: string) {
  return render(
    <DataGrid source={source} headerDisplay="twoRow" pageSize={200} reloadToken={0} />,
  );
}

describe('DataGrid', () => {
  it('tells the user the staging area is empty', async () => {
    mocked.getGridHeader.mockResolvedValue(header(''));
    mocked.queryData.mockResolvedValue(result([]));

    renderGrid('');

    expect(await screen.findByText('暂无临时数据，请使用工具栏的导入或添加')).toBeTruthy();
    expect(screen.getByText(/共 0 行/)).toBeTruthy();
  });

  it('says a committed week has no data, not that staging is empty', async () => {
    mocked.getGridHeader.mockResolvedValue(header('2639', '2639'));
    mocked.queryData.mockResolvedValue(result([]));

    renderGrid('2639');

    expect(await screen.findByText('该周暂无数据')).toBeTruthy();
  });

  it('renders the rows the backend returns and counts them', async () => {
    mocked.getGridHeader.mockResolvedValue(header('2639', '2639'));
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001'), row(2, 'ITEM-0002')]));

    renderGrid('2639');

    // antd renders fixed columns twice, so a value can legitimately appear more
    // than once.
    expect((await screen.findAllByText('ITEM-0001')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('ITEM-0002').length).toBeGreaterThan(0);
    expect(screen.getByText(/共 2 行/)).toBeTruthy();
  });

  it('shows the empty state instead of stale rows when the query fails', async () => {
    mocked.getGridHeader.mockResolvedValue(header('2639', '2639'));
    mocked.queryData.mockRejectedValue(new Error('backend unavailable'));

    renderGrid('2639');

    expect(await screen.findByText('该周暂无数据')).toBeTruthy();
    expect(screen.getByText(/共 0 行/)).toBeTruthy();
  });

  it('keeps the newer result when an older request finishes later', async () => {
    // A slow first response must not overwrite the rows of the request that
    // superseded it — the page-size selector and the search box both make this
    // reachable in normal use.
    let releaseFirst: (r: GridResult) => void = () => {};
    const first = new Promise<GridResult>((resolve) => {
      releaseFirst = resolve;
    });

    mocked.getGridHeader.mockResolvedValue(header('2639', '2639'));
    mocked.queryData.mockReturnValueOnce(first);
    mocked.queryData.mockResolvedValue(result([row(2, 'NEWER')], 1));

    const view = renderGrid('2639');
    // Wait until the first request is genuinely in flight before superseding it;
    // rerendering earlier would simply cancel it and there would be nothing to
    // race against.
    await vi.waitFor(() => expect(mocked.queryData).toHaveBeenCalledTimes(1));
    view.rerender(
      <DataGrid source="2640" headerDisplay="twoRow" pageSize={200} reloadToken={0} />,
    );

    expect((await screen.findAllByText('NEWER')).length).toBeGreaterThan(0);

    releaseFirst(result([row(1, 'OLDER')], 1));
    // Give the stale response a chance to land before checking it did not.
    await new Promise((r) => setTimeout(r, 20));

    expect(screen.queryByText('OLDER')).toBeNull();
    expect(screen.getAllByText('NEWER').length).toBeGreaterThan(0);
  });
  it('returns to the first page and re-queries when the page size changes', async () => {
    // The kernel owns the page size now, so this covers the wiring: the footer's
    // size changer must reach it, and a bigger page must not leave the user on
    // page 3 of a list that no longer has one.
    mocked.getGridHeader.mockResolvedValue(header('2639', '2639'));
    mocked.queryData.mockResolvedValue({ ...result([row(1, 'ITEM-0001')]), total: 900 });

    renderGrid('2639');
    await screen.findAllByText('ITEM-0001');

    fireEvent.click(await screen.findByTitle('2'));
    await waitFor(() =>
      expect(mocked.queryData).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, pageSize: 200 })),
    );

    fireEvent.mouseDown(screen.getByRole('combobox'));
    fireEvent.click(await screen.findByText('500 / page'));

    await waitFor(() =>
      expect(mocked.queryData).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, pageSize: 500 })),
    );
  });
});
