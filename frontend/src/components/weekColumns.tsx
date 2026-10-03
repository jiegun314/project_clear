import { Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { MessageSquare } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { formatQuantity, formatWeekStart, rawValueHint } from '../lib/format';
import { WEEK_WIDTH } from '../lib/gridWidth';
import type { GridRow, Week } from '../types';

export { WEEK_WIDTH };

/** The week block, shared by the main grid and the history view. */
export function buildWeekColumns(
  weeks: Week[],
  headerDisplay: 'twoRow' | 'oneRow',
): ColumnsType<GridRow> {
  return weeks.map((w, i) => ({
    title: (
      <div style={{ lineHeight: 1.15, textAlign: 'center' }}>
        <div style={{ fontWeight: 600, color: JNJ.ink }}>{w.code}</div>
        {headerDisplay === 'twoRow' && (
          <div
            style={{
              fontSize: 11,
              color: JNJ.textMuted,
              fontWeight: 400,
              whiteSpace: 'nowrap',
            }}
          >
            {formatWeekStart(w.start)}
          </div>
        )}
      </div>
    ),
    dataIndex: ['weeks', i],
    key: `w${w.code}`,
    width: WEEK_WIDTH,
    align: 'right' as const,
    // The source cell's own fill colour is carried through the database, so a
    // row looks the way it looked in the workbook. It is painted on the cell
    // content rather than through `onCell`: WKWebView (the macOS shell) drops
    // the style antd puts on <td>, while Chromium keeps it, so `onCell` alone
    // showed colours in the browser harness but not in the application.
    onCell: (row: GridRow) => {
      const meta = row.weekMeta?.[i];
      return meta?.color ? { style: { background: meta.color } } : {};
    },
    render: (v: string, row: GridRow) => {
      const meta = row.weekMeta?.[i];
      const text = formatQuantity(v);
      const n = Number((v ?? '').trim());
      const numeric = Number.isFinite(n);
      // The stored value is a tooltip rather than the display, so a rounded
      // cell can still be traced back to the number in the workbook.
      const hint = rawValueHint(v, text);
      const color = !numeric ? undefined : n < 0 ? JNJ.danger : n === 0 ? JNJ.textMuted : JNJ.ink;
      return (
        <span
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: 4,
            // Cover the whole cell, padding included, without changing the row
            // height: the negative margin cancels antd's 8px cell padding.
            margin: '-8px -8px',
            padding: '8px 8px',
            minHeight: 22,
            background: meta?.color,
            boxSizing: 'content-box',
          }}
        >
          <span style={{ color, fontVariantNumeric: 'tabular-nums' }} title={hint || undefined}>
            {text}
          </span>
          {meta?.comment && (
            <Tooltip
              title={
                <div style={{ maxWidth: 320 }}>
                  {meta.author && (
                    <div style={{ fontWeight: 600, marginBottom: 2 }}>{meta.author}</div>
                  )}
                  <div style={{ whiteSpace: 'pre-wrap' }}>{meta.comment}</div>
                </div>
              }
            >
              <MessageSquare
                size={12}
                style={{ color: JNJ.warning, flex: '0 0 auto' }}
                aria-label="批注"
              />
            </Tooltip>
          )}
        </span>
      );
    },
  }));
}

/** Total pixel width of the week block, for the table's horizontal scroll. */
export function weekBlockWidth(weeks: Week[]): number {
  return weeks.length * WEEK_WIDTH;
}
