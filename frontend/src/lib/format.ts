/**
 * Display formatting for the data grids.
 *
 * Everything the planner reads is a whole quantity, so numbers are rounded and
 * grouped; a zero means "nothing to plan", which is shown as a dash rather than
 * a wall of 0s. The database and the exported workbook keep the full precision
 * — only the screen is rounded.
 */

/** The placeholder for a zero or empty cell. */
export const DASH = '-';

/**
 * The week start as YYYY/MM/DD.
 *
 * The value arrives as a Go time.Time, i.e. "2026-09-21T00:00:00Z", which is
 * far wider than the column header it goes in. Only the date part is shown; a
 * value that is already short is handed back untouched.
 */
export function formatWeekStart(value: string | null | undefined): string {
  const text = (value ?? '').trim();
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(text);
  if (!match) return text;
  return `${match[1]}/${match[2]}/${match[3]}`;
}

/** Round half away from zero, matching 四舍五入 (Math.round(-0.5) is -0, not -1). */
export function roundHalfAwayFromZero(n: number): number {
  return n < 0 ? -Math.round(-n) : Math.round(n);
}

/** True when the cell text is a number rather than text. */
export function isNumericCell(raw: string | null | undefined): boolean {
  const text = (raw ?? '').trim();
  if (text === '') return false;
  return Number.isFinite(Number(text));
}

/**
 * Formats a quantity for the grid: an integer with thousands separators, or a
 * dash when the value is zero. Anything that is not a number — a part code, a
 * description — is handed back exactly as it came, so nothing is silently
 * rewritten into something it is not.
 */
export function formatQuantity(raw: string | null | undefined): string {
  const text = (raw ?? '').trim();
  if (text === '') return DASH;
  const n = Number(text);
  if (!Number.isFinite(n)) return raw ?? '';
  const rounded = roundHalfAwayFromZero(n);
  if (rounded === 0) return DASH;
  return rounded.toLocaleString('en-US');
}

/**
 * Index columns that hold a quantity rather than a label. The A..O block is
 * otherwise made of codes and descriptions: "MFG CLASS CODE" is numeric-looking
 * but is an identifier, and grouping it into 6,803 would change its meaning.
 */
const QUANTITY_INDEX_COLUMNS = new Set([12, 13]); // M = BO, N = OH

/** Formats one cell of the fixed A..O index block. */
export function formatIndexCell(columnIndex: number, raw: string | null | undefined): string {
  if (QUANTITY_INDEX_COLUMNS.has(columnIndex)) return formatQuantity(raw);
  const text = (raw ?? '').trim();
  return text === '' ? DASH : text;
}

/**
 * The value to show when the rounded number is not the whole story. Returns ""
 * when the display already matches the stored value, so no needless tooltip is
 * attached.
 */
export function rawValueHint(raw: string | null | undefined, shown: string): string {
  const text = (raw ?? '').trim();
  if (text === '' || text === shown) return '';
  return text;
}
