import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { indexSortKey, weekSortKey } from './sortKeys.ts';

// The store accepts c1..c15 and wk_<code> and nothing else: an unrecognised key
// is replaced with "seq" without any error, so the header keeps its sort arrow
// while the rows stay in insertion order. These tests pin the wire format.
describe('indexSortKey', () => {
  it('numbers the A..O block from one, the way the store names its columns', () => {
    assert.equal(indexSortKey(0), 'c1');
    assert.equal(indexSortKey(1), 'c2');
    assert.equal(indexSortKey(14), 'c15');
  });

  it('never produces the zero-based key that the store rejects', () => {
    // c0 is outside c1..c15, so it fell through to seq and the first column
    // silently did not sort.
    for (let i = 0; i < 15; i++) {
      assert.notEqual(indexSortKey(i), 'c0');
      assert.match(indexSortKey(i), /^c([1-9]|1[0-5])$/);
    }
  });

  it('keeps the last column inside the accepted range', () => {
    // INDEX_COLS is 15; c16 would be rejected as out of range.
    assert.equal(indexSortKey(15), 'c16');
  });
});

describe('weekSortKey', () => {
  it('uses the wk_ prefix the store looks for', () => {
    assert.equal(weekSortKey('2639'), 'wk_2639');
    assert.equal(weekSortKey('2701'), 'wk_2701');
  });

  it('never produces the bare w<code> form that the store ignores', () => {
    // "w2639" does not start with "wk_", so every week header used to sort by
    // seq while antd still painted the arrow.
    assert.notEqual(weekSortKey('2639'), 'w2639');
    assert.ok(weekSortKey('2639').startsWith('wk_'));
  });
});
