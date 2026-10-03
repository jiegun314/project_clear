/**
 * The log feed is classified by the level the backend stamps on every entry.
 * The panel uses that to draw one column per type and to show how many entries
 * of each type the buffer holds, so the split happens automatically instead of
 * the user having to eyeball the text.
 */

export const LOG_LEVELS = ['info', 'success', 'warn', 'error'] as const;

export type LogLevel = (typeof LOG_LEVELS)[number];

export const LEVEL_TAG: Record<LogLevel, string> = {
  info: '信息',
  success: '成功',
  warn: '警告',
  error: '错误',
};

export interface LevelCounts extends Record<LogLevel, number> {
  all: number;
}

function isLogLevel(level: string): level is LogLevel {
  return (LOG_LEVELS as readonly string[]).includes(level);
}

/** levelTag falls back to the raw level so an unknown value is still visible. */
export function levelTag(level: string): string {
  return isLogLevel(level) ? LEVEL_TAG[level] : level;
}

/** countByLevel tallies the live feed for the filter chips. */
export function countByLevel(entries: readonly { level: string }[]): LevelCounts {
  const counts: LevelCounts = { all: entries.length, info: 0, success: 0, warn: 0, error: 0 };
  for (const entry of entries) {
    if (entry && isLogLevel(entry.level)) counts[entry.level] += 1;
  }
  return counts;
}

/** filterByLevel keeps 'all' meaning "no filter at all". */
export function filterByLevel<T extends { level: string }>(entries: readonly T[], level: string): T[] {
  if (level === 'all') return entries.slice();
  return entries.filter((e) => e.level === level);
}
