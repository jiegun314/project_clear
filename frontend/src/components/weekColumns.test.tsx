import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { buildWeekColumns } from './weekColumns';
import type { Week } from '../types';

// The settings screen now exposes `headerDisplay`, so what the two modes actually
// mean is pinned here: the two-row header carries the week code and its start
// date, and the one-row header drops the date rather than folding it onto one line.

const WEEKS: Week[] = [{ code: '2639', year: 26, weekNo: 39, start: '2026-09-21' }];

function headerHtml(display: 'twoRow' | 'oneRow'): string {
  const columns = buildWeekColumns(WEEKS, display);
  return renderToStaticMarkup(<>{columns[0].title as React.ReactElement}</>);
}

describe('buildWeekColumns', () => {
  it('shows the week code and its start date in the two-row header', () => {
    const html = headerHtml('twoRow');
    expect(html).toContain('2639');
    expect(html).toContain('2026/09/21');
  });

  it('shows only the week code in the one-row header', () => {
    const html = headerHtml('oneRow');
    expect(html).toContain('2639');
    expect(html).not.toContain('2026/09/21');
  });
});
