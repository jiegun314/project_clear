import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ImportResultModal } from './ImportResultModal';
import type { FileResult, ImportResult } from '../types';

function file(name: string, status: string): FileResult {
  return { name, path: `/in/${name}`, size: 1024, status, rowsTotal: 10, rowsKept: 4, weekCode: '2639' };
}

const BASE: ImportResult = {
  action: 'import', weekCode: '2639', weekStart: '2026-09-21', weekCodes: [], indexNames: [],
  total: 2, ok: 1, failed: 1, rowsKept: 8, durationMs: 120,
  files: [file('good.xlsm', 'ok'), file('bad.xlsm', 'failed')],
  warnings: [], draft: true, datasetId: 7, localRows: 0, version: 0, expired: false,
  rowCount: 8, fileCount: 1,
};

describe('ImportResultModal', () => {
  it('explains that a Global import is still a draft', () => {
    render(<ImportResultModal open result={BASE} onClose={() => {}} />);
    expect(screen.getByText(/已加入 Global 导入草稿/)).toBeTruthy();
    expect(screen.getByText('这是 Global 导入草稿')).toBeTruthy();
    expect(screen.getByText('导入行数')).toBeTruthy();
    expect(screen.getByText('8')).toBeTruthy();
  });

  it('reports a Local import with its version and an expired integration warning', () => {
    const local: ImportResult = {
      ...BASE, action: 'local', total: 1, ok: 1, failed: 0, rowsKept: 1736,
      draft: false, localRows: 1736, version: 4, expired: true, datasetId: 9,
    };
    render(<ImportResultModal open result={local} onClose={() => {}} />);
    expect(screen.getByText('Local 导入完成')).toBeTruthy();
    expect(screen.getByText('Local v4')).toBeTruthy();
    expect(screen.getByText('该周已有整合记录，现在已过期')).toBeTruthy();
  });
});
