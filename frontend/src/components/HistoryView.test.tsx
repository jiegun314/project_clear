import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { HistoryView } from './HistoryView';
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

const header: GridHeader = {
  datasetId: 9, kind: 'merged', version: 3, weekCode: '2639', weekStart: '2026-09-21',
  indexNames: INDEX_NAMES,
  weeks: [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }],
  total: 0, expired: false,
};

function row(seq: number, item: string): GridRow {
  return {
    seq,
    index: ['P5', 'P4', 'P3', 'P2', 'P1', 'GROUP A', 'CLASS 1', 'DESCR', item,
      'US-1', 'EU-1', 'WH_CNB', '0', '0', 'AdjDmd'],
    weeks: ['120'], weekMeta: [{}], fileName: 'synthetic.xlsm', srcRow: 57 + seq,
  };
}

const result = (rows: GridRow[]): GridResult => ({ rows, total: rows.length, page: 1, pageSize: 500 });

function renderHistory(
  datasetId: number | null,
  onReady?: () => void,
  onExport = vi.fn(async () => null),
  onExportBundle = vi.fn(async () => null),
  dataset = 'merged',
) {
  return render(
    <HistoryView
      datasetId={datasetId}
      dataset={dataset}
      onClose={() => {}}
      onExported={() => {}}
      onExport={onExport}
      onExportBundle={onExportBundle}
      onReady={onReady}
    />,
  );
}

describe('HistoryView', () => {
  it('renders the dataset rows and names its version', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001'), row(2, 'ITEM-0002')]));

    renderHistory(9);

    expect((await screen.findAllByText('ITEM-0001')).length).toBeGreaterThan(0);
    expect(screen.getByText(/共 2 行/)).toBeTruthy();
    expect(screen.getByText('v3')).toBeTruthy();
    expect(mocked.queryData).toHaveBeenCalledWith(expect.objectContaining({ datasetId: 9 }));
  });

  it('says the dataset has no data', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([]));

    renderHistory(9);

    expect(await screen.findByText('该周暂无数据')).toBeTruthy();
  });

  it('warns that the integrated week is expired', async () => {
    mocked.getGridHeader.mockResolvedValue({ ...header, expired: true });
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001')]));
    renderHistory(9);
    expect(await screen.findByText(/该周已有更新的导入，整合已过期/)).toBeTruthy();
  });

  it('signals readiness once per dataset', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001')]));
    const onReady = vi.fn();

    const view = renderHistory(9, onReady);
    await vi.waitFor(() => expect(onReady).toHaveBeenCalled());

    view.rerender(
      <HistoryView
        datasetId={9}
        onClose={() => {}}
        onExported={() => {}}
        onExport={async () => null}
        onExportBundle={async () => null}
        onReady={onReady}
      />,
    );
    await new Promise((r) => setTimeout(r, 20));
    expect(onReady).toHaveBeenCalledTimes(1);

    fireEvent.change(screen.getByPlaceholderText('搜索索引列'), { target: { value: 'ITEM' } });
    await waitFor(() => expect(mocked.queryData).toHaveBeenCalledTimes(2), { timeout: 2000 });
    await new Promise((r) => setTimeout(r, 50));
    expect(onReady).toHaveBeenCalledTimes(1);
  });

  it('exports the merged dataset as clean MPS or as the coloured three-table file', async () => {
    mocked.getGridHeader.mockResolvedValue(header);
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001')]));
    const onExport = vi.fn(async () => null);
    const onExportBundle = vi.fn(async () => null);

    renderHistory(9, undefined, onExport, onExportBundle);
    await screen.findAllByText('ITEM-0001');

    // The old template (.xlsm) export is gone from the merged row; the
    // coloured three-table export replaces it.
    expect(screen.queryByRole('button', { name: /原文件格式/ })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /导出纯数据/ }));
    await waitFor(() => expect(onExport).toHaveBeenCalledWith(9, 'clean'));

    fireEvent.click(screen.getByRole('button', { name: /导出三表（带颜色）/ }));
    await waitFor(() => expect(onExportBundle).toHaveBeenCalledWith('2639'));
    // The bundle uses the week, so it still works when no workbench session is open.
    expect(onExport).toHaveBeenCalledTimes(1);
  });

  it('keeps the original-format export for Global/Local input datasets', async () => {
    mocked.getGridHeader.mockResolvedValue({ ...header, kind: 'global' });
    mocked.queryData.mockResolvedValue(result([row(1, 'ITEM-0001')]));
    const onExport = vi.fn(async () => null);
    const onExportBundle = vi.fn(async () => null);

    renderHistory(9, undefined, onExport, onExportBundle, 'global');
    await screen.findAllByText('ITEM-0001');
    expect(screen.queryByRole('button', { name: /导出三表（带颜色）/ })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /导出原文件格式/ }));
    await waitFor(() => expect(onExport).toHaveBeenCalledWith(9, 'template'));
  });
});
