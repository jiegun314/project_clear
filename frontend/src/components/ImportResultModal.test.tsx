import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { ImportResultModal } from './ImportResultModal';
import { JNJ } from '../theme/jnj';
import type { FileResult, ImportResult } from '../types';

// The status chip on the import summary. The requirement is a light grey fill
// with the success green as the text, centred in its cell both ways — asserted
// through the inline style, because that is where the requirement lives. The old
// version used the preset `color="success"`, which fills the tag with a pale tint
// derived from the same dark green and reads as grey rather than as "it worked".

/** jsdom normalises hex colours to rgb() when they are read back. */
function toRgb(hex: string): string {
  const n = parseInt(hex.slice(1), 16);
  return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`;
}

function file(name: string, status: string): FileResult {
  return { name, path: `/in/${name}`, size: 1024, status, rowsTotal: 10, rowsKept: 4, weekCode: '2639' };
}

const RESULT: ImportResult = {
  action: 'import',
  weekCode: '2639',
  weekStart: '2026-09-21',
  weekCodes: [],
  indexNames: [],
  total: 2,
  ok: 1,
  failed: 1,
  rowsKept: 8,
  durationMs: 120,
  files: [file('good.xlsm', 'ok'), file('bad.xlsm', 'failed')],
  warnings: [],
};

/**
 * The status chip for one row. antd splits a scrolling table into a separate
 * header and body table, so the chip is found by its own text instead: the other
 * tags in this dialog read 周码 …, 起始日期 … and 周列 … 个.
 */
function chipInTable(label: string): HTMLElement {
  const chip = Array.from(document.querySelectorAll('.ant-tag')).find(
    (t) => t.textContent?.trim() === label,
  );
  if (!chip) throw new Error(`no tag reading ${label}`);
  return chip as HTMLElement;
}

function renderModal() {
  return render(<ImportResultModal open result={RESULT} onClose={() => {}} />);
}

describe('ImportResultModal', () => {
  it('draws 成功 as the success green on the lightest grey fill', () => {
    renderModal();
    const chip = chipInTable('成功');

    expect(chip.style.background).toBe(toRgb(JNJ.fill));
    expect(chip.style.color).toBe(toRgb(JNJ.success));
    // antd's tag margin would push the chip off centre inside a centred cell.
    expect(chip.style.margin).toBe('0px');
  });

  it('centres the status cell horizontally and vertically', () => {
    renderModal();
    const cell = chipInTable('成功').closest('td') as HTMLElement;

    // `align="center"` gives the horizontal half; a table cell's default
    // vertical-align is baseline, so the vertical half has to be asked for.
    expect(cell.style.textAlign).toBe('center');
    expect(cell.style.verticalAlign).toBe('middle');
  });

  it('still marks a failed file, in a cell centred the same way', () => {
    renderModal();
    const chip = chipInTable('失败');
    const cell = chip.closest('td') as HTMLElement;

    expect(cell.style.textAlign).toBe('center');
    expect(cell.style.verticalAlign).toBe('middle');
  });
});
