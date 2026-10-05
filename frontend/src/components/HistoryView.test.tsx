import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { HistoryView } from './HistoryView';
import type { GridHeader, GridResult, GridRow } from '../types';

// The archive window is the second half of the duplicated table logic, and it is
// the one that used to re-query on every parent render. These tests cover the
// states a user actually sees — data, no data, and the one-shot "first load is
// done" signal the parent uses to drop its loading cover.

vi.mock('../services/api', () => ({
  api: { getGridHeader: vi.fn(), queryData: vi.fn() },
}));

const { api } = await import('../services/api');
const mocked = vi.mocked(api);

const INDEX_NAMES = [
  'P5', 'P4', 'P3', 'P2', 'P1', 'MFG GROUP', 'MFG CLASS CODE', 'MFG DESCR',
  'ITEM', 'US CATALOG', 'EU CATALOG', 'LOC', 'BO', 'OH', 'LOC',
];

const header: GridHeader = {
  source: '2639',
  weekCode: '2639',
  weekStart: '2026-09-21',
  indexNames: INDEX_NAMES,
  weeks: [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }],
  total: 0,
  hasStaging: false,
};

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

function result(rows: GridRow[]): GridResult {
  return { rows, total: rows.length, page: 1, pageSize: 500 };
}

function renderHistory(weekCode: string | null, onReady?: () => void) {
  return render(
    <HistoryView
      weekCode={weekCode}
      onClose={() => {}}
      onExported={() => {}}
      onExport={async () => null}
      onReady={onReady}
    />,
  );
}

describe('HistoryView', () => {
  it('renders the rows of the week and counts them', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001'), row(2, 'ITEM-0002')]));

    renderHistory('2639');

    expect((await screen.findAllByText('ITEM-0001')).length).toBeGreaterThan(0);
    expect(screen.getByText(/共 2 行/)).toBeTruthy();
  });

  it('says the week has no data rather than showing nothing', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([]));

    renderHistory('2639');

    expect(await screen.findByText('该周暂无数据')).toBeTruthy();
  });

  it('signals readiness once per week, and not again when the same week is re-read', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001')]));
    const onReady = vi.fn();

    const view = renderHistory('2639', onReady);
    await vi.waitFor(() => expect(onReady).toHaveBeenCalled());

    // Re-rendering the same week must not fire it again: the parent would keep
    // re-showing a loading cover, which is the bug this signal exists to avoid.
    view.rerender(
      <HistoryView
        weekCode="2639"
        onClose={() => {}}
        onExported={() => {}}
        onExport={async () => null}
        onReady={onReady}
      />,
    );
    await new Promise((r) => setTimeout(r, 20));
    expect(onReady).toHaveBeenCalledTimes(1);

    // Nor may a second read of the same week: flipping through pages or typing in
    // the search box re-queries, and the cover would come back every time.
    fireEvent.change(screen.getByPlaceholderText('搜索索引列'), { target: { value: 'ITEM' } });
    await waitFor(() => expect(mocked.queryData).toHaveBeenCalledTimes(2), { timeout: 2000 });
    await new Promise((r) => setTimeout(r, 50));

    expect(onReady).toHaveBeenCalledTimes(1);
  });

  it('keeps showing the rows it has when a later read fails', async () => {
    // The archive window deliberately differs from the main grid here: the grid
    // clears its rows on a failed read, this window keeps them. The behaviour is
    // preserved by an option on the shared table kernel, and pinned here so the
    // option cannot be dropped as "unused".
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValueOnce(result([row(1, 'ITEM-0001')]));

    renderHistory('2639');
    expect((await screen.findAllByText('ITEM-0001')).length).toBeGreaterThan(0);

    mocked.queryData.mockRejectedValue(new Error('backend unavailable'));
    fireEvent.change(screen.getByPlaceholderText('搜索索引列'), { target: { value: 'ITEM' } });

    // Past the search debounce, so the failing read has certainly happened.
    await waitFor(() => expect(mocked.queryData).toHaveBeenCalledTimes(2), { timeout: 2000 });
    await new Promise((r) => setTimeout(r, 50));

    expect(screen.queryByText('该周暂无数据')).toBeNull();
    expect(screen.getAllByText('ITEM-0001').length).toBeGreaterThan(0);
  });
});
