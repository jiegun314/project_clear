import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { HistoryPanel } from './HistoryPanel';
import type { ArchiveEntry } from '../types';

vi.mock('../services/api', () => ({
  api: { getYears: vi.fn(), getWeeks: vi.fn(), getArchive: vi.fn() },
}));
const { api } = await import('../services/api');
const mocked = vi.mocked(api);

function entry(
  dataset: string,
  id: number,
  rows: number,
  expired = false,
  weekCode = '2639',
  weekNo = 39,
): ArchiveEntry {
  return {
    weekCode, weekStart: weekNo === 40 ? '2026-09-28' : '2026-09-21',
    year: 2026, weekNo, dataset, datasetId: id, version: 2, rowCount: rows, fileCount: 13,
    committedAt: '2026-10-10 21:06:55', expired,
  };
}

beforeEach(() => {
  mocked.getYears.mockResolvedValue([2026]);
  mocked.getWeeks.mockResolvedValue([39]);
  mocked.getArchive.mockResolvedValue([
    entry('merged', 100, 2580),
    entry('global', 101, 1516),
    entry('local', 102, 1736),
  ]);
});

describe('HistoryPanel', () => {
  it('lists every dataset of a week and opens the one that is clicked', async () => {
    const onView = vi.fn();
    render(<HistoryPanel onView={onView} reloadToken={0} />);

    expect(await screen.findByText('整合')).toBeTruthy();
    expect(screen.getByText('Global')).toBeTruthy();
    expect(screen.getByText('Local')).toBeTruthy();
    expect(screen.getByText(/2,580 行/)).toBeTruthy();
    expect(screen.getByText(/1,516 行/)).toBeTruthy();
    expect(screen.getByText(/1,736 行/)).toBeTruthy();
    expect(screen.getAllByText(/创建于/)).toHaveLength(3);

    fireEvent.click(screen.getByText('Global').closest('div[style*="cursor"]') as HTMLElement);
    await waitFor(() => expect(onView).toHaveBeenCalledWith(101, 'global'));
  });

  it('opens the week\'s merged dataset from the 查看 button', async () => {
    const onView = vi.fn();
    render(<HistoryPanel onView={onView} reloadToken={0} />);
    await screen.findByText('整合');
    fireEvent.click(screen.getByRole('button', { name: /查看/ }));
    await waitFor(() => expect(onView).toHaveBeenCalledWith(100, 'merged'));
  });

  it('marks the datasets of an expired integration', async () => {
    mocked.getArchive.mockResolvedValue([entry('merged', 100, 2580, true)]);
    render(<HistoryPanel onView={() => {}} reloadToken={0} />);
    expect(await screen.findByText('该周整合已过期')).toBeTruthy();
    expect(screen.getByText(/v2/)).toBeTruthy();
  });

  it('follows the workbench to a newly imported week', async () => {
    mocked.getArchive.mockResolvedValue([
      entry('merged', 200, 2580, false, '2640', 40),
      entry('global', 201, 1516, false, '2640', 40),
      entry('merged', 100, 2580, false, '2639', 39),
    ]);
    mocked.getWeeks.mockResolvedValue([40, 39]);
    const onView = vi.fn();

    render(<HistoryPanel onView={onView} reloadToken={1} activeWeek="2640" />);

    // The selectors and 查看 target follow the new week, not the previously
    // selected 2639.
    expect(await screen.findByText('第 40 周')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /查看/ }));
    await waitFor(() => expect(onView).toHaveBeenCalledWith(200, 'merged'));
  });

  it('lists a week whose imports are complete but not integrated yet', async () => {
    mocked.getArchive.mockResolvedValue([
      entry('global', 101, 1516),
      entry('local', 102, 1736),
    ]);
    const onView = vi.fn();
    render(<HistoryPanel onView={onView} reloadToken={0} />);

    expect(await screen.findByText('Global')).toBeTruthy();
    expect(screen.getByText('Local')).toBeTruthy();
    expect(screen.queryByText('整合')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /查看/ }));
    await waitFor(() => expect(onView).toHaveBeenCalledWith(101, 'global'));
  });

  it('marks only the integration row as expired; Global and Local stay current', async () => {
    // After a new Global import the history's Global row moves to the new
    // version, while the integration — and only the integration — is outdated.
    mocked.getArchive.mockResolvedValue([
      entry('merged', 100, 2580, true),
      entry('global', 103, 1516, false),
      entry('local', 102, 1736, false),
    ]);
    render(<HistoryPanel onView={() => {}} reloadToken={0} />);
    await screen.findByText('该周整合已过期');
    expect(screen.getAllByText('已过期')).toHaveLength(1);
  });
});
