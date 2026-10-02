import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Table, Input, Select, Space, Tag, Tooltip, Empty, Segmented } from 'antd';
import type { ColumnsType, TableProps } from 'antd/es/table';
import { Search, RotateCw } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { GridHeader, GridRow, Week } from '../types';

export interface DataGridProps {
  /** "" shows the staging area; a week code shows that committed week. */
  source: string;
  headerDisplay: 'twoRow' | 'oneRow';
  pageSize: number;
  /** Bumped by the parent after an import or commit to force a reload. */
  reloadToken: number;
}

const INDEX_WIDTHS = [110, 130, 150, 150, 170, 90, 100, 180, 150, 130, 130, 90, 80, 90, 90];

function numberCell(v: string) {
  if (v === '' || v === null || v === undefined) return '';
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  // Keep the workbook's own precision, but trim the noise a planner never
  // wants to read (2499.9999999999995 -> 2500 is NOT done; only .0 is).
  return Number.isInteger(n) ? String(n) : v;
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
  const reqId = useRef(0);

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
        pageSize,
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
  }, [header, source, page, pageSize, debounced, sort]);

  useEffect(() => {
    void load();
  }, [load]);

  const indexCols = useMemo<ColumnsType<GridRow>>(() => {
    const names = header?.indexNames ?? [];
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
    const weeks: Week[] = header?.weeks ?? [];
    return weeks.map((w, i) => ({
      title: (
        <div style={{ lineHeight: 1.15, textAlign: 'center' }}>
          <div style={{ fontWeight: 600, color: JNJ.ink }}>{w.code}</div>
          {headerDisplay === 'twoRow' && (
            <div style={{ fontSize: 11, color: JNJ.textMuted, fontWeight: 400 }}>{w.start}</div>
          )}
        </div>
      ),
      dataIndex: ['weeks', i],
      key: `w${w.code}`,
      width: 96,
      align: 'right',
      render: (v: string) => {
        const text = numberCell(v ?? '');
        if (text === '') return <span style={{ color: JNJ.textMuted }}>—</span>;
        const n = Number(v);
        const color =
          text === '' ? undefined : n < 0 ? JNJ.danger : n === 0 ? JNJ.textMuted : JNJ.ink;
        return <span style={{ color, fontVariantNumeric: 'tabular-nums' }}>{text}</span>;
      },
    }));
  }, [header, headerDisplay]);

  const allColumns = [...indexCols, ...weekCols];

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
          共 {total.toLocaleString()} 行 · {header?.weeks.length ?? 0} 个周列
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

      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', background: JNJ.surface }}>
        <Table<GridRow>
          size="small"
          bordered
          sticky
          rowKey="seq"
          loading={loading}
          columns={columns}
          dataSource={rows}
          scroll={{ x: 'max-content', y: 'calc(100vh - 520px)' }}
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
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            pageSizeOptions: [50, 100, 200, 500, 1000],
            showTotal: (t, r) => `第 ${r[0]}-${r[1]} 条 / 共 ${t.toLocaleString()} 条`,
          }}
          onChange={(pagination, filters, sorter, extra) => {
            onTableChange(pagination, filters, sorter, extra);
            setPage(pagination.current ?? 1);
          }}
        />
      </div>
    </div>
  );
}
