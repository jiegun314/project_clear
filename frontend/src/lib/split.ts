/**
 * The two halves of the main window — the merged data grid above, the history
 * and log panels below — are separated by a bar the user can drag.
 *
 * The limits are taken from the height the lower half has when the program
 * starts, so they mean the same thing at whatever size the window is later
 * dragged to: the lower half may grow to 120% of its start-up height and shrink
 * to 80%, no further.
 */

/** The draggable bar between the two halves, in pixels. */
export const SPLIT_BAR = 8;
/** The lower half may be dragged between 80% and 120% of its start-up height. */
export const SPLIT_MIN_RATIO = 0.8;
export const SPLIT_MAX_RATIO = 1.2;
/** The data grid keeps at least this much room, whatever the drag does. */
export const TOP_MIN_HEIGHT = 200;

export interface SplitBounds {
  min: number;
  max: number;
}

/** The limits for a lower half that starts out `startHeight` pixels tall. */
export function splitBounds(startHeight: number): SplitBounds {
  return {
    min: Math.round(startHeight * SPLIT_MIN_RATIO),
    max: Math.round(startHeight * SPLIT_MAX_RATIO),
  };
}

/**
 * The height the lower half should take for a proposed value. It never leaves
 * the start-up limits, and it never squeezes the grid below its own minimum —
 * unless the window is so short that the two cannot both hold, in which case
 * the explicit limit wins.
 */
export function clampLower(value: number, bounds: SplitBounds, containerHeight: number): number {
  const room = containerHeight - TOP_MIN_HEIGHT - SPLIT_BAR;
  const ceiling = Math.max(bounds.min, Math.min(bounds.max, room));
  return Math.round(Math.min(Math.max(value, bounds.min), ceiling));
}

/** The lower height after the bar is dragged `dy` pixels (negative is up). */
export function lowerAfterDrag(startHeight: number, dy: number): number {
  return startHeight - dy;
}
