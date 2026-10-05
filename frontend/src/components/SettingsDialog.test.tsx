import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SettingsDialog } from './SettingsDialog';
import type { AppConfig, ConfigView } from '../types';

// The parameter screen as it behaves today, so that making the form declarative
// does not quietly drop a field, change a control or lose the failure states that
// were added when a failed load used to leave the window blank.

vi.mock('../services/api', () => ({
  api: { getConfig: vi.fn(), saveConfig: vi.fn(), resetConfig: vi.fn() },
}));

const { api } = await import('../services/api');
const mocked = vi.mocked(api);

const CONFIG: AppConfig = {
  configVersion: 2,
  readColumns: 20,
  locFilter: 'WH_CNB',
  pageSize: 200,
  headerDisplay: 'twoRow',
  exportMode: 'clean',
  exportDir: '',
};

const VIEW: ConfigView = { config: CONFIG, path: '/tmp/config/clear.yaml' };

const SAVE = '保存参数';
const RESET = '恢复默认参数';

function renderDialog(over: { onSaved?: (c: AppConfig) => void } = {}) {
  return render(<SettingsDialog open onClose={() => {}} onSaved={over.onSaved ?? (() => {})} />);
}

beforeEach(() => {
  mocked.getConfig.mockResolvedValue(VIEW);
  mocked.saveConfig.mockResolvedValue([]);
  mocked.resetConfig.mockResolvedValue(VIEW);
});

describe('SettingsDialog', () => {
  it('shows every parameter with the value the backend holds', async () => {
    renderDialog();

    // Labels.
    expect(await screen.findByText('读取列数（从 P 列开始）')).toBeTruthy();
    expect(screen.getByText('LOC 筛选值（L 列）')).toBeTruthy();
    expect(screen.getByText('每页显示行数')).toBeTruthy();
    expect(screen.getByText('导出方式')).toBeTruthy();

    // The two numeric controls, in order, with their current values.
    const numbers = screen.getAllByRole('spinbutton') as HTMLInputElement[];
    expect(numbers.map((n) => n.value)).toEqual(['20', '200']);

    // The text control.
    expect((screen.getByRole('textbox') as HTMLInputElement).value).toBe('WH_CNB');

    // The two export modes, with 纯数据 selected.
    const radios = screen.getAllByRole('radio') as HTMLInputElement[];
    expect(radios.map((r) => r.checked)).toEqual([true, false]);

    // And the path of the file these are written to.
    expect(screen.getByText(/\/tmp\/config\/clear\.yaml/)).toBeTruthy();
  });

  it('sends the edited values when saved', async () => {
    renderDialog();
    await screen.findByText('读取列数（从 P 列开始）');

    const numbers = screen.getAllByRole('spinbutton') as HTMLInputElement[];
    fireEvent.change(numbers[0], { target: { value: '30' } });
    fireEvent.blur(numbers[0]);
    fireEvent.click(screen.getByRole('radio', { name: /原文件格式/ }));
    fireEvent.click(screen.getByRole('button', { name: SAVE }));

    await waitFor(() =>
      expect(mocked.saveConfig).toHaveBeenCalledWith(
        expect.objectContaining({ readColumns: 30, exportMode: 'template' }),
      ),
    );
  });

  it('tells the user when the parameters could not be read', async () => {
    mocked.getConfig.mockRejectedValue(new Error('磁盘已满'));
    renderDialog();

    expect(await screen.findByText('参数读取失败：磁盘已满')).toBeTruthy();
    // Nothing to edit, and no save button to press on a form that never loaded.
    expect(screen.queryAllByRole('spinbutton')).toHaveLength(0);
  });

  it('asks the backend to reset and reports what happened', async () => {
    const onSaved = vi.fn();
    renderDialog({ onSaved });
    await screen.findByText('读取列数（从 P 列开始）');

    fireEvent.click(screen.getByRole('button', { name: RESET }));

    await waitFor(() => expect(mocked.resetConfig).toHaveBeenCalled());
    expect(await screen.findByText('已恢复出厂默认参数')).toBeTruthy();
    expect(onSaved).toHaveBeenCalledWith(CONFIG);
  });
  it('shows the explanation under every parameter that has one', async () => {
    // The wording lives in the schema now, so this checks it still reaches the
    // screen rather than being dropped on the way through the renderer.
    renderDialog();
    await screen.findByText('读取列数（从 P 列开始）');

    expect(screen.getByText('源文件第 55/56 行的周码与起始日期列数，默认 20，即 P..AI')).toBeTruthy();
    expect(screen.getByText('只保留该 LOC 的数据行，留空将回退为 WH_CNB')).toBeTruthy();
    expect(screen.getByText('两种方式都只输出 MPS 页，都保留颜色、条件格式与备注')).toBeTruthy();
  });
});

