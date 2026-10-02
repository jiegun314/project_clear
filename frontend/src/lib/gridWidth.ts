/** Width of one week column; the grid, the history view and the tests share it. */
export const WEEK_WIDTH = 96;

/**
 * Widths of the frozen index columns. They add up to a fixed block so the
 * header table and the body table agree on the column grid.
 */
export const INDEX_WIDTHS = [110, 130, 150, 150, 170, 90, 100, 180, 150, 130, 130, 90, 80, 90, 90];

export const INDEX_BLOCK_WIDTH = INDEX_WIDTHS.reduce((a, b) => a + b, 0);

/**
 * Horizontal scroll width for the integration grid. `undefined` disables
 * horizontal scrolling: a grid without columns has nothing to scroll, and
 * pinning it to the index-block width painted a pointless scrollbar under the
 * empty placeholder.
 */
export function gridTableWidth(columnCount: number, weekCount: number): number | undefined {
  if (columnCount <= 0) return undefined;
  return INDEX_BLOCK_WIDTH + weekCount * WEEK_WIDTH;
}
