import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Toolbar } from './Toolbar';
import type { StagingFilesView } from '../types';

// The toolbar had no test of its own until the antd 6 prop renames: `Divider`'s
// `type` became `orientation`, which is only visible in what the toolbar renders,
// and nothing else in the suite looks at it.
//
// The data actions are grouped by input (Global / Local / 合并与产出 / 清单), and
// 整合 waits for both inputs — both are asserted here.

const COMMIT = '整合：合并 Global 与 Local，并写入永久周数据表';
const EXPORT = '导出：整合后生成 MPS / Local / Global 三表文件';
const IMPORT_LOCAL = '导入本地 MPS 文件（Month End）：检查格式并写入 Local 数据';

function noop() {}

function renderToolbar(over: Partial<Parameters<typeof Toolbar>[0]> = {}) {
  return render(
    <Toolbar
      busy={false}
      hasStaging={true}
      hasGlobal={true}
      hasLocal={true}
      hasArchive={true}
      stagedFiles={null}
      onImport={noop}
      onAdd={noop}
      onImportLocal={noop}
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
  it('separates its four action groups with vertical dividers', () => {
    const { container } = renderToolbar();

    // Global / Local / 合并与产出 / 清单: three separators between the groups
    // plus the one after the logo.
    const dividers = Array.from(container.querySelectorAll('[role="separator"]'));
    expect(dividers).toHaveLength(4);
    for (const d of dividers) {
      expect(d.className).toContain('ant-divider-vertical');
    }
  });

  it('offers the local import as its own action', () => {
    const onImportLocal = vi.fn();
    renderToolbar({ onImportLocal });
    fireEvent.click(screen.getByRole('button', { name: IMPORT_LOCAL }));
    expect(onImportLocal).toHaveBeenCalled();
  });

  it('waits for both inputs before 整合 can run', () => {
    const { unmount } = renderToolbar({ hasStaging: false, hasGlobal: false, hasLocal: false });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(true);
    unmount();

    const missingLocal = renderToolbar({ hasGlobal: true, hasLocal: false });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(true);
    missingLocal.unmount();

    renderToolbar({ hasGlobal: true, hasLocal: true });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(false);
  });

  it('offers 导出 only once the week is integrated', () => {
    const { unmount } = renderToolbar({ hasStaging: true, hasArchive: true });
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(true);
    unmount();

    const noArchive = renderToolbar({ hasStaging: false, hasArchive: false });
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(true);
    noArchive.unmount();

    renderToolbar({ hasStaging: false, hasArchive: true });
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(false);
  });

  it('reports the current workbench behind the 已导入文件 button', () => {
    const staged: StagingFilesView = {
      hasStaging: true,
      fileCount: 14,
      rowCount: 3252,
      failedCount: 0,
      batchState: 'staging',
      files: [],
    };
    const onViewStagedFiles = vi.fn();

    renderToolbar({ stagedFiles: staged, onViewStagedFiles });
    fireEvent.click(screen.getByRole('button', { name: '查看已导入文件' }));

    expect(onViewStagedFiles).toHaveBeenCalled();
    // While the workbench is open the count says it is waiting for 整合, rather
    // than showing a row total that is neither side's count.
    expect(screen.getByText('14 个文件 · 待整合')).toBeTruthy();
  });
});
