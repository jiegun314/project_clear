import assert from 'node:assert/strict';
import { describe, it } from 'vitest';

import { INDEX_BLOCK_WIDTH, WEEK_WIDTH, gridTableWidth } from './gridWidth.ts';

describe('gridTableWidth', () => {
  it('does not force horizontal scrolling while the grid is empty', () => {
    // No header means no columns; a pixel width here is what produced the
    // pointless scrollbar under the "暂无数据" placeholder.
    assert.equal(gridTableWidth(0, 0), undefined);
  });

  it('keeps the index block and the week columns on one pixel grid', () => {
    assert.equal(gridTableWidth(15, 3), INDEX_BLOCK_WIDTH + 3 * WEEK_WIDTH);
  });
});
