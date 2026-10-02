import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { Table, Input, Pagination, Space, Tag, Tooltip, Empty } from 'antd';
import type { ColumnsType, TableProps } from 'antd/es/table';
import { Search, RotateCw } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import { INDEX_BLOCK_WIDTH, INDEX_WIDTHS, gridTableWidth } from '../lib/gridWidth';
import { buildWeekColumns } from './weekColumns';
import type { GridHeader, GridRow } from '../types';

export interface DataGridProps {
  /** "" shows the staging area; a week code shows that committed week. */
  source: string;
  headerDisplay: 'twoRow' | 'oneRow';
  pageSize: number;
  /** Bumped by the parent after an import or commit to force a reload. */
  reloadToken: number;
}

export function DataGrid({ source, headerDisplay, pageSize, reloadToken }: DataGridProps) {
  const [header, setHeader] = useState<GridHeader | null>(null);
  const [rows, setRows] = useState<GridRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [debounced, setDebounced] = useState('');
  const [sort, setSort] = useState<{ field: string; desc: boolean }>({ field: 'seq', desc: false });
  const [loading, setLoading] = useState(false);
  // The page size starts from the configured default but the footer's size
  // selector owns it from then on.
  const [size, setSize] = useState(pageSize);
  useEffect(() => setSize(pageSize), [pageSize]);
  const reqId = useRef(0);
  // The table body follows the space the splitter gives it. antd wants a
  // number, so the wrapper and its separate sticky header are measured.
  const scrollRef = useRef<HTMLDivElement>(null);
  const [bodyHeight, setBodyHeight] = useState(0);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const update = () => {
      const header = el.querySelector('.ant-table-header') as HTMLElement | null;
      const headerHeight = header ? header.offsetHeight : 48;
      setBodyHeight(Math.max(160, el.clientHeight - headerHeight));
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(el);
    const header = el.querySelector('.ant-table-header');
    if (header) observer.observe(header);
    return () => observer.disconnect();
  }, [header]);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search.trim()), 250);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    setPage(1);
  }, [source, debounced, sort.field, sort.desc, reloadToken]);

  useEffect(() => {
    let alive = true;
    api
      .getGridHeader(source)
      .then((h) => {
        if (alive) setHeader(h);
      })
      .catch(() => {
        if (alive) setHeader(null);
      });
    return () => {
      alive = false;
    };
  }, [source, reloadToken]);

  const load = useCallback(async () => {
    if (!header) return;
    const id = ++reqId.current;
    setLoading(true);
    try {
      const res = await api.queryData({
        source,
        page,
        pageSize: size,
        search: debounced,
        sortField: sort.field,
        sortDesc: sort.desc,
        filters: {},
      });
      // A slower earlier request must not overwrite a newer one.
      if (id !== reqId.current) return;
      setRows(res.rows ?? []);
      setTotal(res.total ?? 0);
    } catch (e) {
      if (id === reqId.current) {
        setRows([]);
        setTotal(0);
      }
    } finally {
      if (id === reqId.current) setLoading(false);
    }
  }, [header, source, page, size, debounced, sort]);

  useEffect(() => {
    void load();
  }, [load]);

  const indexCols = useMemo<ColumnsType<GridRow>>(() => {
    const names: string[] = header?.indexNames ?? [];
    return names.map((name, i) => ({
      title: (
        <Tooltip title={`第 ${i + 1} 列 · 固定索引`}>
          <span>{name || `列${i + 1}`}</span>
        </Tooltip>
      ),
      dataIndex: ['index', i],
      key: `c${i}`,
      width: INDEX_WIDTHS[i] ?? 120,
      fixed: i === 8 ? 'left' : undefined,
      ellipsis: true,
      render: (v: string, row) => {
        const text = v === '' ? '—' : v;
        return (
          <Tooltip title={row.fileName ? `${row.fileName} · 源第 ${row.srcRow} 行` : undefined}>
            <span style={{ color: v === '' ? JNJ.textMuted : undefined }}>{text}</span>
          </Tooltip>
        );
      },
    }));
  }, [header]);

  const weekCols = useMemo<ColumnsType<GridRow>>(() => {
    // Go marshals a nil slice as null, so the array is never assumed here.
    return buildWeekColumns(header?.weeks ?? [], headerDisplay);
  }, [header, headerDisplay]);

  const allColumns = [...indexCols, ...weekCols];
  // An explicit pixel width keeps the header table and the body table on the
  // same grid; `max-content` let each of them solve the layout on its own.
  // Before anything is imported there are no columns at all, and forcing the
  // pixel width would paint a meaningless horizontal scrollbar under the empty
  // placeholder.
  const tableWidth = gridTableWidth(allColumns.length, header?.weeks?.length ?? 0);

  // Sorting is a backend concern: the columns only declare that they are
  // sortable and report the intent, and the order flag is echoed back so the
  // header shows the current direction.
  const columns = allColumns.map((c) => {
    const key = String(c.key);
    return {
      ...c,
      sorter: true,
      sortOrder: sort.field === key ? (sort.desc ? ('descend' as const) : ('ascend' as const)) : null,
    };
  }) as ColumnsType<GridRow>;

  const onTableChange: TableProps<GridRow>['onChange'] = (_pagination, _filters, sorter) => {
    const s = Array.isArray(sorter) ? sorter[0] : sorter;
    const key = String(s?.columnKey ?? '');
    if (s?.order && key) {
      setSort({ field: key, desc: s.order === 'descend' });
    } else {
      setSort({ field: 'seq', desc: false });
    }
  };

  const title = source === '' ? '整合数据（临时）' : `整合数据（周码 ${source}）`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
      <div
        style={{
          background: JNJ.red,
          color: '#fff',
          padding: '5px 12px',
          fontWeight: 600,
          fontSize: 13,
          letterSpacing: 0.5,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
        }}
      >
        <span>{title}</span>
        {header?.weekCode && (
          <Tag
            style={{
              background: 'rgba(255,255,255,0.22)',
              color: '#fff',
              border: 'none',
              margin: 0,
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {header.weekCode} · {header.weekStart}
          </Tag>
        )}
        <span style={{ marginLeft: 'auto', fontWeight: 400, opacity: 0.92, fontSize: 12 }}>
          共 {total.toLocaleString()} 行 · {header?.weeks?.length ?? 0} 个周列
        </span>
      </div>

      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '6px 10px',
          background: JNJ.surface,
          borderBottom: `1px solid ${JNJ.divider}`,
        }}
      >
        <Input
          allowClear
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          prefix={<Search size={14} style={{ color: JNJ.textMuted }} />}
          placeholder="搜索 P1..P5 / 物料 / 描述 / 目录 ..."
          style={{ width: 300 }}
        />
        <Space style={{ marginLeft: 'auto' }}>
          <Tooltip title="刷新">
            <a onClick={() => void load()} style={{ color: JNJ.text }}>
              <RotateCw size={15} className={loading ? 'spin' : undefined} />
            </a>
          </Tooltip>
        </Space>
      </div>

      {/* One scrollbar only: the table body scrolls inside this box, while the
          pagination sits below it and never scrolls out of view. */}
      <div ref={scrollRef} style={{ flex: 1, minHeight: 0, overflow: 'hidden', background: JNJ.surface }}>
        <Table<GridRow>
          size="small"
          bordered
          sticky
          tableLayout="fixed"
          rowKey="seq"
          loading={loading}
          columns={columns}
          dataSource={rows}
          pagination={false}
          scroll={{ x: tableWidth, y: bodyHeight > 0 ? bodyHeight : 'calc(100vh - 520px)' }}
          locale={{
            emptyText: (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={
                  source === '' ? '暂无临时数据，请使用工具栏的导入或添加' : '该周暂无数据'
                }
              />
            ),
          }}
          onChange={(pagination, filters, sorter, extra) => onTableChange(pagination, filters, sorter, extra)}
        />
      </div>

      <div
        style={{
          flex: '0 0 auto',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          padding: '6px 10px',
          background: JNJ.surface,
          borderTop: `1px solid ${JNJ.border}`,
        }}
      >
        <Pagination
          size="small"
          current={page}
          pageSize={size}
          total={total}
          showSizeChanger
          pageSizeOptions={[50, 100, 200, 500, 1000]}
          showTotal={(t, range) => `第 ${range[0]}-${range[1]} 条 / 共 ${t.toLocaleString()} 条`}
          onChange={(nextPage, nextSize) => {
            if (nextSize !== size) setSize(nextSize);
            setPage(nextSize !== size ? 1 : nextPage);
          }}
        />
      </div>
    </div>
  );
}
