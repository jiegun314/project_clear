import { Tooltip } from 'antd';
import { JNJ } from '../theme/jnj';
import type { ColumnsType } from 'antd/es/table';
import { INDEX_WIDTHS, gridTableWidth } from './gridWidth';
import { formatIndexCell } from './format';
import { indexSortKey } from './sortKeys';
import { buildWeekColumns } from '../components/weekColumns';
import type { GridHeader, GridRow } from '../types';
import type { SortState } from '../hooks/useTableQuery';

export interface GridColumnOptions {
  header: GridHeader | null;
  /** The main grid can collapse the two header rows; the archive window never does. */
  headerDisplay: 'twoRow' | 'oneRow';
  sort: SortState;
  /** The tooltip over an index column's title. */
  indexTitle: (columnNumber: number) => string;
  /**
   * The tooltip over a single index cell. The main grid uses it to name the
   * workbook and source row a value came from; the archive window shows nothing.
   */
  cellTitle?: (row: GridRow) => string | undefined;
}

/**
 * The columns both tables render: the fixed A..O index block followed by the
 * week columns, each declared sortable and showing the current direction.
 *
 * Index columns 9 (ITEM) and 15 (the second LOC) stay pinned to the left so they
 * remain visible when the table is scrolled sideways.
 */
export function buildGridColumns(opt: GridColumnOptions): {
  columns: ColumnsType<GridRow>;
  /** Undefined when there are no columns at all, which disables horizontal scrolling. */
  tableWidth: number | undefined;
} {
  const names: string[] = opt.header?.indexNames ?? [];
  const indexCols: ColumnsType<GridRow> = names.map((name, i) => ({
    title: (
      <Tooltip title={opt.indexTitle(i + 1)}>
        <span>{name || `列${i + 1}`}</span>
      </Tooltip>
    ),
    dataIndex: ['index', i],
    key: indexSortKey(i),
    width: INDEX_WIDTHS[i] ?? 120,
    fixed: i === 8 || i === 14 ? 'left' : undefined,
    ellipsis: true,
    render: (v: string, row) => {
      const label = opt.cellTitle?.(row);
      const cell = (
        <span style={{ color: v === '' ? JNJ.textMuted : undefined }}>{formatIndexCell(i, v)}</span>
      );
      return label ? <Tooltip title={label}>{cell}</Tooltip> : cell;
    },
  }));

  const all = [...indexCols, ...buildWeekColumns(opt.header?.weeks ?? [], opt.headerDisplay)];
  const columns = all.map((c) => {
    const key = String(c.key);
    return {
      ...c,
      sorter: true,
      sortOrder: opt.sort.field === key ? (opt.sort.desc ? ('descend' as const) : ('ascend' as const)) : null,
    };
  }) as ColumnsType<GridRow>;

  // An explicit pixel width keeps the header table and the body table on the same
  // grid; `max-content` let each of them solve the layout on its own.
  return { columns, tableWidth: gridTableWidth(columns.length, opt.header?.weeks?.length ?? 0) };
}
