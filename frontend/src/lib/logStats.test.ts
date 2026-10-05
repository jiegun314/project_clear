import assert from 'node:assert/strict';
import { describe, it } from 'vitest';

import { countByLevel, filterByLevel, levelTag } from './logStats.ts';

describe('levelTag', () => {
  it('names the four levels the backend emits', () => {
    assert.equal(levelTag('info'), '信息');
    assert.equal(levelTag('success'), '成功');
    assert.equal(levelTag('warn'), '警告');
    assert.equal(levelTag('error'), '错误');
  });

  it('keeps an unknown level visible instead of blanking it', () => {
    assert.equal(levelTag('debug'), 'debug');
  });
});

describe('countByLevel', () => {
  it('splits the feed by type and totals everything under all', () => {
    const entries = [
      { level: 'info' },
      { level: 'info' },
      { level: 'success' },
      { level: 'warn' },
      { level: 'error' },
      { level: 'error' },
      { level: 'error' },
    ];
    assert.deepEqual(countByLevel(entries), {
      all: 7,
      info: 2,
      success: 1,
      warn: 1,
      error: 3,
    });
  });

  it('ignores an entry whose level is not one of the four', () => {
    assert.deepEqual(countByLevel([{ level: 'info' }, { level: 'debug' }]), {
      all: 2,
      info: 1,
      success: 0,
      warn: 0,
      error: 0,
    });
  });

  it('reports zeroes for an empty feed', () => {
    assert.deepEqual(countByLevel([]), { all: 0, info: 0, success: 0, warn: 0, error: 0 });
  });
});

describe('filterByLevel', () => {
  const entries = [{ level: 'info' }, { level: 'error' }, { level: 'info' }];

  it('returns every entry for all', () => {
    assert.equal(filterByLevel(entries, 'all').length, 3);
  });

  it('keeps only the requested type', () => {
    assert.deepEqual(filterByLevel(entries, 'info'), [{ level: 'info' }, { level: 'info' }]);
    assert.deepEqual(filterByLevel(entries, 'warn'), []);
  });
});
