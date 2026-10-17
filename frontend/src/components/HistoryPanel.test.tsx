import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { HistoryPanel } from './HistoryPanel';
import type { ArchiveEntry } from '../types';

// A committed week now holds three datasets, so the lower-left list shows one row
// per dataset and opening one opens exactly that dataset — this is how the global
// rows, the local rows and the merged result stay separable after 整合.

vi.mock('../services/api', () => ({
  api: { getYears: vi.fn(), getWeeks: vi.fn(), getArchive: vi.fn() },
}));
const { api } = await import('../services/api');
const mocked = vi.mocked(api);

function entry(dataset: string, rows: number): ArchiveEntry {
  return {
    weekCode: '2639', weekStart: '2026-09-21', year: 2026, weekNo: 39,
    weekCodes: ['2639'], indexNames: [], rowCount: rows, fileCount: 13,
    batchId: 1, committedAt: '2026-10-10 21:06:55',
    tableName: dataset === 'merged' ? 'data_2639' : `data_2639_${dataset}`,
    dataset,
  };
}

beforeEach(() => {
  mocked.getYears.mockResolvedValue([2026]);
  mocked.getWeeks.mockResolvedValue([39]);
  mocked.getArchive.mockResolvedValue([
    entry('merged', 2580),
    entry('global', 1516),
    entry('local', 1736),
  ]);
});

describe('HistoryPanel', () => {
  it('lists every dataset of a week and opens the one that is clicked', async () => {
    const onView = vi.fn();
    render(<HistoryPanel onView={onView} reloadToken={0} />);

    expect(await screen.findByText('整合')).toBeTruthy();
    expect(screen.getByText('Global')).toBeTruthy();
    expect(screen.getByText('Local')).toBeTruthy();
    // Each row carries its own row count, so the three are distinguishable.
    expect(screen.getByText(/2,580 行/)).toBeTruthy();
    expect(screen.getByText(/1,516 行/)).toBeTruthy();
    expect(screen.getByText(/1,736 行/)).toBeTruthy();

    // The dataset name is a tag at the head of the row, and every row says when
    // it was created (not when it was integrated — the same row covers both).
    expect(document.querySelectorAll('.ant-tag')).toHaveLength(3);
    expect(screen.getAllByText(/创建于/)).toHaveLength(3);
    expect(screen.queryByText(/整合于/)).toBeNull();

    fireEvent.click(screen.getByText('Global').closest('div[style*="cursor"]') as HTMLElement);
    await waitFor(() => expect(onView).toHaveBeenCalledWith('2639', 'global'));
  });
});
