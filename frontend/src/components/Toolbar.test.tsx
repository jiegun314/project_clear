import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Toolbar } from './Toolbar';

const COMMIT = '整合：合并已完成的 Global 与 Local，写入永久整合记录';
const EXPORT = '导出：生成 MPS / Local / Global 三表文件';
const EXPORT_EXPIRED = '导出：生成三表文件（当前整合已过期，导出的是上次整合结果）';
const FINISH = '完成 Global 导入：把当前草稿定稿为可整合的数据集';
const IMPORT_LOCAL = '导入本地 MPS 文件（Month End）：检查格式并写入 Local 数据集';
const CLEAR = '清空：丢弃未完成的 Global 导入';

function noop() {}

function renderToolbar(over: Partial<Parameters<typeof Toolbar>[0]> = {}) {
  return render(
    <Toolbar
      busy={false}
      hasDraft={false}
      hasGlobal={true}
      hasLocal={true}
      canIntegrate={true}
      hasIntegration={true}
      integrationExpired={false}
      countLabel="13 个文件 · 已整合"
      failedCount={0}
      onImport={noop}
      onAdd={noop}
      onFinish={noop}
      onImportLocal={noop}
      onCommit={noop}
      onClear={noop}
      onExport={noop}
      onViewFiles={noop}
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
    const dividers = Array.from(container.querySelectorAll('[role="separator"]'));
    expect(dividers).toHaveLength(4);
    for (const d of dividers) expect(d.className).toContain('ant-divider-vertical');
  });

  it('only enables 完成 Global 导入 while a draft is open', () => {
    const { unmount } = renderToolbar({ hasDraft: false });
    expect(screen.getByRole('button', { name: FINISH }).hasAttribute('disabled')).toBe(true);
    unmount();

    const onFinish = vi.fn();
    renderToolbar({ hasDraft: true, onFinish });
    const finish = screen.getByRole('button', { name: FINISH });
    expect(finish.hasAttribute('disabled')).toBe(false);
    fireEvent.click(finish);
    expect(onFinish).toHaveBeenCalled();
  });

  it('only enables 整合 when both inputs are complete and no draft is open', () => {
    const { unmount } = renderToolbar({ hasDraft: true, hasGlobal: true, hasLocal: true, canIntegrate: false });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(true);
    unmount();

    const missing = renderToolbar({ hasGlobal: true, hasLocal: false, canIntegrate: false });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(true);
    missing.unmount();

    renderToolbar({ canIntegrate: true });
    expect(screen.getByRole('button', { name: COMMIT }).hasAttribute('disabled')).toBe(false);
  });

  it('offers 导出 only once the week is integrated, and says when it is expired', () => {
    const { unmount } = renderToolbar({ hasIntegration: false });
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(true);
    unmount();

    const expired = renderToolbar({ hasIntegration: true, integrationExpired: true });
    expect(screen.getByRole('button', { name: EXPORT_EXPIRED }).hasAttribute('disabled')).toBe(false);
    expired.unmount();

    renderToolbar({ hasIntegration: true, integrationExpired: false });
    expect(screen.getByRole('button', { name: EXPORT }).hasAttribute('disabled')).toBe(false);
  });

  it('shows the file count and only allows discarding a draft', () => {
    const onViewFiles = vi.fn();
    const onClear = vi.fn();
    const { unmount } = renderToolbar({ countLabel: '14 个文件 · 导入中', onViewFiles, onClear });
    expect(screen.getByText('14 个文件 · 导入中')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: '查看 Global 已导入文件' }));
    expect(onViewFiles).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: CLEAR }).hasAttribute('disabled')).toBe(true);
    unmount();

    renderToolbar({ hasDraft: true, countLabel: '14 个文件 · 导入中', onClear });
    const clear = screen.getByRole('button', { name: CLEAR });
    expect(clear.hasAttribute('disabled')).toBe(false);
    fireEvent.click(clear);
    expect(onClear).toHaveBeenCalled();
  });

  it('calls the local import action', () => {
    const onImportLocal = vi.fn();
    renderToolbar({ onImportLocal });
    fireEvent.click(screen.getByRole('button', { name: IMPORT_LOCAL }));
    expect(onImportLocal).toHaveBeenCalled();
  });
});
