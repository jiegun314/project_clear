import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StagedFilesModal } from './StagedFilesModal';
import type { FileWithDataset, FilesView } from '../types';

function file(status: string): FileWithDataset {
  return {
    name: 'source.xlsm', path: '/in/source.xlsm', size: 1024, status,
    rowsTotal: 10, rowsKept: 4, weekCode: '2639', dataset: 'Global', version: 0,
  };
}

function view(hasDraft: boolean, status = 'ok'): FilesView {
  return {
    files: [file(status)], hasDraft, fileCount: 1, rowCount: 4, failedCount: status === 'ok' ? 0 : 1,
  };
}

describe('StagedFilesModal', () => {
  it('shows Global on the left and 导入中 on the right while the import is a draft', () => {
    render(<StagedFilesModal open view={view(true)} onClose={() => {}} />);
    expect(screen.getByText('Global')).toBeTruthy();
    expect(screen.getByText('导入中')).toBeTruthy();
    expect(screen.queryByText('已导入')).toBeNull();
  });

  it('shows 已导入 once the Global import is complete', () => {
    render(<StagedFilesModal open view={view(false)} onClose={() => {}} />);
    expect(screen.getByText('Global')).toBeTruthy();
    expect(screen.getByText('已导入')).toBeTruthy();
    expect(screen.queryByText('导入中')).toBeNull();
  });

  it('still marks a failed workbook as 读取失败', () => {
    render(<StagedFilesModal open view={view(true, 'failed')} onClose={() => {}} />);
    expect(screen.getByText('读取失败')).toBeTruthy();
  });
});
