/**
 * A readable message for anything a rejected promise can carry.
 *
 * Wails rejects with the Go error's text, but a webview-side throw arrives as a
 * real Error and anything else as whatever was thrown, so all three shapes have
 * to be handled in one place instead of each caller guessing.
 */
export function errorText(e: unknown): string {
  if (typeof e === 'string') return e;
  if (e instanceof Error) return e.message;
  return String(e);
}
