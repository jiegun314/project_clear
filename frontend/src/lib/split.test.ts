import assert from 'node:assert/strict';
import { describe, it } from 'vitest';

import { clampLower, lowerAfterDrag, splitBounds, SPLIT_BAR, TOP_MIN_HEIGHT } from './split.ts';

describe('splitBounds', () => {
  it('is 80%..120% of the start-up height', () => {
    assert.deepEqual(splitBounds(400), { min: 320, max: 480 });
    assert.deepEqual(splitBounds(333), { min: 266, max: 400 });
  });
});

describe('clampLower', () => {
  const bounds = splitBounds(400);

  it('holds the lower half inside its limits', () => {
    assert.equal(clampLower(1000, bounds, 900), 480, 'dragged far up');
    assert.equal(clampLower(10, bounds, 900), 320, 'dragged far down');
    assert.equal(clampLower(410, bounds, 900), 410, 'inside the range');
  });

  it('never squeezes the grid below its own minimum', () => {
    // In a 600px container the lower half may take at most
    // 600 - TOP_MIN_HEIGHT - SPLIT_BAR, which is less than its own maximum.
    const container = 600;
    const room = container - TOP_MIN_HEIGHT - SPLIT_BAR;
    assert.ok(room < bounds.max, 'the test needs a container that is tight');
    assert.equal(clampLower(1000, bounds, container), room);
  });

  it('prefers the explicit minimum when the window is very short', () => {
    assert.equal(clampLower(400, bounds, 300), 320);
  });
});

describe('lowerAfterDrag', () => {
  it('moves the divider with the pointer', () => {
    assert.equal(lowerAfterDrag(400, -100), 500, 'dragging up grows the lower half');
    assert.equal(lowerAfterDrag(400, 60), 340);
  });
});
