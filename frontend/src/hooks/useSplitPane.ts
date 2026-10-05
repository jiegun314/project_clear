import { useCallback, useEffect, useRef, useState } from 'react';
import { SPLIT_BAR, clampLower, lowerAfterDrag, splitBounds, type SplitBounds } from '../lib/split';

/** The start-up height is kept alongside the limits, so a double click can go back to it. */
export interface SplitLimits extends SplitBounds {
  start: number;
}

export interface SplitPane {
  /** The container the two halves live in. */
  splitRef: React.RefObject<HTMLDivElement | null>;
  /** The lower half, whose start-up height defines the limits. */
  lowerRef: React.RefObject<HTMLDivElement | null>;
  /** The height the lower half should take, or null while it is still the flex split. */
  lowerPx: number | null;
  limits: SplitLimits | null;
  dragging: boolean;
  startResize: (e: React.MouseEvent) => void;
  nudge: (e: React.KeyboardEvent) => void;
  /** Back to the start-up split. */
  reset: () => void;
}

/**
 * The resizable split between the merged data and the panels underneath.
 *
 * The limits are measured once, from the height the lower half has at start-up,
 * so they keep meaning the same thing after the window is dragged to another
 * size. Null means "still the flex split the stylesheet asks for".
 */
export function useSplitPane(): SplitPane {
  const splitRef = useRef<HTMLDivElement>(null);
  const lowerRef = useRef<HTMLDivElement>(null);
  const [lowerHeight, setLowerHeight] = useState<number | null>(null);
  const [limits, setLimits] = useState<SplitLimits | null>(null);
  const [dragging, setDragging] = useState(false);

  // Measured once, from the height the lower half has at start-up. The observer
  // stays until the first real measurement lands, which is not guaranteed on the
  // very first paint.
  useEffect(() => {
    if (limits) return;
    const el = lowerRef.current;
    if (!el) return;
    const measure = () => {
      const start = el.offsetHeight;
      if (start <= 0) return;
      setLimits({ ...splitBounds(start), start });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [limits]);

  // Locked to the height it started with: growing the window hands the extra
  // room to the data grid, and the panels keep their size. Only a drag (or a
  // window too short to hold both) changes it.
  const lowerPx = lowerHeight ?? limits?.start ?? null;

  // Keep the split honest when the window shrinks under it.
  useEffect(() => {
    if (!limits) return;
    const fit = () => {
      const container = splitRef.current?.clientHeight ?? 0;
      if (container > 0) setLowerHeight((h) => clampLower(h ?? limits.start, limits, container));
    };
    window.addEventListener('resize', fit);
    return () => window.removeEventListener('resize', fit);
  }, [limits]);

  const startResize = useCallback(
    (e: React.MouseEvent) => {
      const container = splitRef.current;
      const lower = lowerRef.current;
      if (!container || !lower || !limits) return;
      e.preventDefault();
      // WebKit does not focus a non-form element on click, so the arrow keys
      // would have nothing to act on. Focusing the handle makes the keyboard
      // route work as well as the drag.
      (e.currentTarget as HTMLElement).focus();
      const from = e.clientY;
      const startedAt = lowerHeight ?? lower.offsetHeight;
      const containerHeight = container.clientHeight;
      setDragging(true);

      const move = (ev: MouseEvent) => {
        setLowerHeight(clampLower(lowerAfterDrag(startedAt, ev.clientY - from), limits, containerHeight));
      };
      const stop = () => {
        window.removeEventListener('mousemove', move);
        window.removeEventListener('mouseup', stop);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        setDragging(false);
      };
      window.addEventListener('mousemove', move);
      window.addEventListener('mouseup', stop);
      document.body.style.cursor = 'row-resize';
      document.body.style.userSelect = 'none';
    },
    [limits, lowerHeight],
  );

  const nudge = useCallback(
    (e: React.KeyboardEvent) => {
      if (!limits) return;
      const container = splitRef.current?.clientHeight ?? 0;
      const step = e.key === 'ArrowUp' ? 16 : e.key === 'ArrowDown' ? -16 : 0;
      if (step === 0) return;
      e.preventDefault();
      const current = lowerHeight ?? lowerRef.current?.offsetHeight ?? limits.min;
      setLowerHeight(clampLower(current + step, limits, container));
    },
    [limits, lowerHeight],
  );

  const reset = useCallback(() => setLowerHeight(null), []);

  return { splitRef, lowerRef, lowerPx, limits, dragging, startResize, nudge, reset };
}

export { SPLIT_BAR };
