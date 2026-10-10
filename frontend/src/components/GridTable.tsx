import { Empty, Spin, Table } from 'antd';
import type { TableProps } from 'antd';
import { JNJ } from '../theme/jnj';
import type { GridRow } from '../types';

export interface GridTableProps {
  rows: GridRow[];
  columns: TableProps<GridRow>['columns'];
  loading: boolean;
  /** Shown in place of the rows when there are none. */
  emptyText: string;
  /** Undefined disables horizontal scrolling, which is what an empty grid wants. */
  scrollX?: number;
  /** How tall the body may grow. Differs by placement, so the caller decides. */
  scrollY: number | string;
  /** The archive window pages through the table; the main grid has its own footer. */
  pagination?: TableProps<GridRow>['pagination'];
  onChange?: TableProps<GridRow>['onChange'];
}

/**
 * The table both screens show. Everything here is what makes them look and
 * behave like the same table — the density, the borders, the sticky header, the
 * fixed layout, the row key and the empty state. The surrounding chrome (title
 * bar, search box, footer, page size control) stays with each screen, because
 * those genuinely differ.
 */
export function GridTable({ rows, columns, loading, emptyText, scrollX, scrollY, pagination, onChange }: GridTableProps) {
  // With nothing to show there is no table: antd would still draw the header row
  // and a bordered body with the placeholder inside them, which reads as a stray box
  // in the upper half of the area. The symbol and its message alone, centred in the
  // whole area, is what an empty list should look like.
  if (rows.length === 0) {
    return (
      <div
        style={{
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: JNJ.surface,
        }}
      >
        {loading ? (
          <Spin />
        ) : (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={<span style={{ color: JNJ.textMuted, fontSize: 13 }}>{emptyText}</span>}
          />
        )}
      </div>
    );
  }

  return (
    <Table<GridRow>
      size="small"
      bordered
      sticky
      tableLayout="fixed"
      rowKey="seq"
      loading={loading}
      columns={columns}
      dataSource={rows}
      scroll={{ x: scrollX, y: scrollY }}
      locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} /> }}
      pagination={pagination ?? false}
      onChange={onChange}
    />
  );
}
