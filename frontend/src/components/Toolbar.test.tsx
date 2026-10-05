import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Toolbar } from './Toolbar';
import type { StagingFilesView } from '../types';

// The toolbar had no test of its own until the antd 6 prop renames: `Divider`'s
// `type` became `orientation`, which is only visible in what the toolbar renders,
// and nothing else in the suite looks at it.

const COMMIT = '整合：将临时数据写入永久周数据表';
const EXPORT = '导出：纯数据或原文件格式（仅 MPS 页）';

function noop() {}

function renderToolbar(over: Partial<Parameters<typeof Toolbar>[0]> = {}) {
  return render(
    <Toolbar
      busy={false}
      hasStaging={false}
      hasArchive={false}
      stagedFiles={null}
      onImport={noop}
      onAdd={noop}
      onCommit={noop}
      onClear={noop}
      onExport={noop}
      onViewStagedFiles={noop}
      onSettings={noop}
      onAbout={noop}
      onQuit={noop}
      {...over}
    />,
  );
}

describe('Toolbar', () => {
  it('separates its groups with vertical dividers', () => {
    const { container } = renderToolbar();

    // Two group separators. `orientation="vertical"` is what makes them vertical:
    // the deprecated `type` prop renders the same markup, so this is the check
    // that the rename kept the toolbar looking as it did.
    const dividers = Array.from(container.querySelectorAll('[role="separator"]'));
    expect(dividers).toHaveLength(2);
    for (const d of dividers) {
      expect(d.className).toContain('ant-divider-vertical');
    }
  });

  it('offers 整合 only with temporary data, and 导出 only with something to export', () => {
    const { unmount } = renderToolbar({ hasStaging: false, hasArchive: false });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(true);
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(true);
    unmount();

    renderToolbar({ hasStaging: true });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(false);
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(false);
  });

  it('reports the current 整合清单 behind the 已导入文件 button', () => {
    const staged: StagingFilesView = {
      hasStaging: true,
      fileCount: 13,
      rowCount: 1516,
      failedCount: 0,
      batchState: 'staging',
      files: [],
    };
    const onViewStagedFiles = vi.fn();

    renderToolbar({ stagedFiles: staged, onViewStagedFiles });
    fireEvent.click(screen.getByRole('button', { name: '查看已导入文件' }));

    expect(onViewStagedFiles).toHaveBeenCalled();
  });
});
