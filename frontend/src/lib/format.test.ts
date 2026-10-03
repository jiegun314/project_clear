import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  DASH,
  formatIndexCell,
  formatWeekStart,
  formatQuantity,
  isNumericCell,
  rawValueHint,
  roundHalfAwayFromZero,
  weekDecimals,
} from './format.ts';

describe('formatWeekStart', () => {
  it('keeps only the date part of a timestamp', () => {
    assert.equal(formatWeekStart('2026-09-21T00:00:00Z'), '2026/09/21');
    assert.equal(formatWeekStart('2026-01-04T00:00:00+08:00'), '2026/01/04');
  });

  it('leaves a plain date alone', () => {
    assert.equal(formatWeekStart('2026-09-21'), '2026/09/21');
  });

  it('handles missing values', () => {
    assert.equal(formatWeekStart(''), '');
    assert.equal(formatWeekStart(null), '');
    assert.equal(formatWeekStart(undefined), '');
  });
});

describe('roundHalfAwayFromZero', () => {
  it('rounds halves away from zero, like 四舍五入', () => {
    assert.equal(roundHalfAwayFromZero(0.5), 1);
    assert.equal(roundHalfAwayFromZero(-0.5), -1);
    assert.equal(roundHalfAwayFromZero(1.5), 2);
    assert.equal(roundHalfAwayFromZero(-1.5), -2);
    assert.equal(roundHalfAwayFromZero(-2.4), -2);
  });
});

describe('formatQuantity', () => {
  it('rounds to whole numbers and groups thousands', () => {
    assert.equal(formatQuantity('899.99999999999977'), '900');
    assert.equal(formatQuantity('1357.0000000000002'), '1,357');
    assert.equal(formatQuantity('1075'), '1,075');
    assert.equal(formatQuantity('1533'), '1,533');
    assert.equal(formatQuantity('1000000'), '1,000,000');
  });

  it('shows a dash for zero, however it is spelled', () => {
    assert.equal(formatQuantity('0'), DASH);
    assert.equal(formatQuantity('0.0'), DASH);
    assert.equal(formatQuantity('-0'), DASH);
    assert.equal(formatQuantity('0.4'), DASH, 'rounds to zero, so it is nothing');
    assert.equal(formatQuantity('-0.4'), DASH);
  });

  it('shows a dash for an empty cell', () => {
    assert.equal(formatQuantity(''), DASH);
    assert.equal(formatQuantity('   '), DASH);
    assert.equal(formatQuantity(null), DASH);
    assert.equal(formatQuantity(undefined), DASH);
  });

  it('keeps negative signs and grouping', () => {
    assert.equal(formatQuantity('-48'), '-48');
    assert.equal(formatQuantity('-1234.6'), '-1,235');
  });

  it('keeps one decimal for the WOS rows', () => {
    assert.equal(formatQuantity('4.9362083383108351', 1), '4.9');
    assert.equal(formatQuantity('5.1158054381973788', 1), '5.1');
    assert.equal(formatQuantity('4.1639569498511566', 1), '4.2');
    assert.equal(formatQuantity('1234.5678', 1), '1,234.6');
    assert.equal(formatQuantity('-0.61224489795918369', 1), '-0.6');
    assert.equal(formatQuantity('5', 1), '5.0', '整数也要补一位小数');
    assert.equal(formatQuantity('0', 1), DASH);
    assert.equal(formatQuantity('0.04', 1), DASH, 'rounds to zero, so it is nothing');
  });

  it('only treats a second LOC of WOS as decimal', () => {
    const index = new Array(15).fill('');
    index[11] = 'LOC';
    index[14] = 'CalcOH';
    assert.equal(weekDecimals(index), 0, '第一列 LOC 不影响');
    index[14] = 'wos';
    assert.equal(weekDecimals(index), 1, '大小写不敏感');
    index[14] = 'WOS ';
    assert.equal(weekDecimals(index), 1, '首尾空格忽略');
    assert.equal(weekDecimals([]), 0);
    assert.equal(weekDecimals(undefined), 0);
  });

  it('leaves text alone', () => {
    assert.equal(formatQuantity('P5_EP_BW'), 'P5_EP_BW');
    assert.equal(formatQuantity('D-1385-01-S-19'), 'D-1385-01-S-19');
    assert.equal(formatQuantity('n/a'), 'n/a');
  });
});

describe('formatIndexCell', () => {
  it('formats the BO/OH quantity columns', () => {
    assert.equal(formatIndexCell(12, '0'), DASH);
    assert.equal(formatIndexCell(13, '6803'), '6,803');
    assert.equal(formatIndexCell(13, '12345.7'), '12,346');
  });

  it('keeps identifier columns exactly as stored', () => {
    // "MFG CLASS CODE" is numeric-looking but must not gain a separator.
    assert.equal(formatIndexCell(6, '6803'), '6803');
    assert.equal(formatIndexCell(0, 'P5_EP_BW'), 'P5_EP_BW');
    assert.equal(formatIndexCell(8, 'D-1385-01-S-19'), 'D-1385-01-S-19');
    assert.equal(formatIndexCell(11, 'WH_CNB'), 'WH_CNB');
  });

  it('shows a dash for empty cells in any column', () => {
    assert.equal(formatIndexCell(0, ''), DASH);
    assert.equal(formatIndexCell(6, '  '), DASH);
    assert.equal(formatIndexCell(12, ''), DASH);
  });
});

describe('isNumericCell', () => {
  it('separates quantities from text', () => {
    assert.equal(isNumericCell('0'), true);
    assert.equal(isNumericCell('-12.5'), true);
    assert.equal(isNumericCell(''), false);
    assert.equal(isNumericCell('  '), false);
    assert.equal(isNumericCell('D138501-19'), false);
  });
});

describe('rawValueHint', () => {
  it('surfaces the stored value only when rounding changed what is shown', () => {
    assert.equal(rawValueHint('899.99999999999977', '900'), '899.99999999999977');
    assert.equal(rawValueHint('1075', '1,075'), '1075');
    assert.equal(rawValueHint('1357', '1,357'), '1357');
    assert.equal(rawValueHint('0', DASH), '0');
    assert.equal(rawValueHint('', DASH), '');
    assert.equal(rawValueHint('P5_EP_BW', 'P5_EP_BW'), '');
  });
});
