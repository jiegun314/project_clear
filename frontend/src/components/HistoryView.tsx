import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Modal, Table, Input, Button, Space, Tag, Empty, Tooltip, Select } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { Search, FileDown, RefreshCw } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { GridHeader, GridRow } from '../types';

const INDEX_WIDTHS = [110, 130, 150, 150, 170, 90, 100, 180, 150, 130, 130, 90, 80, 90, 90];

export interface HistoryViewProps {
  weekCode: string | null;
  onClose: () => void;
  onExported: (msg: string) => void;
}

/**
 * The full table for one committed week. Sorts and filters are done by the
 * backend, but the page size is deliberately larger than the main grid because
 * the brief asks for "all data" here.
 */
export function HistoryView({ weekCode, onClose, onExported }: HistoryViewProps) {
  const [header, setHeader] = useState<GridHeader | null>(null);
  const [rows, setRows] = useState<GridRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(500);
  const [search, setSearch] = useState('');
  const [debounced, setDebounced] = useState('');
  const [sort, setSort] = useState<{ field: string; desc: boolean }>({ field: 'seq', desc: false });
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const reqId = useRef(0);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search.trim()), 250);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    setPage(1);
  }, [weekCode, debounced, sort.field, sort.desc, pageSize]);

  useEffect(() => {
    if (!weekCode) {
      setHeader(null);
      return;
    }
    let alive = true;
    api
      .getGridHeader(weekCode)
      .then((h) => alive && setHeader(h))
      .catch(() => alive && setHeader(null));
    return () => {
      alive = false;
    };
  }, [weekCode]);

  const load = useCallback(async () => {
    if (!weekCode || !header) return;
    const id = ++reqId.current;
    setLoading(true);
    try {
      const res = await api.queryData({
        source: weekCode,
        page,
        pageSize,
        search: debounced,
        sortField: sort.field,
        sortDesc: sort.desc,
        filters: {},
      });
      if (id !== reqId.current) return;
      setRows(res.rows ?? []);
      setTotal(res.total ?? 0);
    } finally {
      if (id === reqId.current) setLoading(false);
    }
  }, [weekCode, header, page, pageSize, debounced, sort]);

  useEffect(() => {
    void load();
  }, [load]);

  const columns = useMemo<ColumnsType<GridRow>>(() => {
    const names: string[] = header?.indexNames ?? [];
    const base: ColumnsType<GridRow> = names.map((name, i) => ({
      title: <Tooltip title={`固定索引 第 ${i + 1} 列`}><span>{name || `列${i + 1}`}</span></Tooltip>,
      dataIndex: ['index', i],
      key: `c${i}`,
      width: INDEX_WIDTHS[i] ?? 120,
      fixed: i === 8 ? 'left' : undefined,
      ellipsis: true,
      render: (v: string) => <span style={{ color: v === '' ? JNJ.textMuted : undefined }}>{v === '' ? '—' : v}</span>,
    }));
    const weekCols: ColumnsType<GridRow> = (header?.weeks ?? []).map((w, i) => ({
      title: (
        <div style={{ lineHeight: 1.15, textAlign: 'center' }}>
          <div style={{ fontWeight: 600, color: JNJ.ink }}>{w.code}</div>
          <div style={{ fontSize: 11, color: JNJ.textMuted, fontWeight: 400 }}>{w.start}</div>
        </div>
      ),
      dataIndex: ['weeks', i],
      key: `w${w.code}`,
      width: 92,
      align: 'right',
      render: (v: string) => <span style={{ fontVariantNumeric: 'tabular-nums' }}>{v === '' ? '—' : v}</span>,
    }));
    return [...base, ...weekCols].map((c) => ({
      ...c,
      sorter: true,
      sortOrder: sort.field === String(c.key) ? (sort.desc ? ('descend' as const) : ('ascend' as const)) : null,
    })) as ColumnsType<GridRow>;
  }, [header, sort]);

  const doExport = async (clean: boolean) => {
    if (!weekCode) return;
    setExporting(true);
    try {
      const res = await api.export(weekCode, clean);
      if (res) onExported(`已导出 ${res.rows} 行 × ${res.cols} 列 → ${res.destPath}`);
    } finally {
      setExporting(false);
    }
  };

  return (
    <Modal
      open={Boolean(weekCode)}
      onCancel={onClose}
      width="94vw"
      style={{ top: 24 }}
      title={
        <Space>
          <span style={{ fontWeight: 600 }}>历史数据</span>
          {header?.weekCode && (
            <>
              <Tag style={{ background: JNJ.red, color: '#fff', border: 'none' }}>
                {header.weekCode}
              </Tag>
              <span style={{ color: JNJ.text, fontWeight: 400, fontSize: 13 }}>{header.weekStart}</span>
            </>
          )}
          <span style={{ color: JNJ.textMuted, fontWeight: 400, fontSize: 12 }}>
            共 {total.toLocaleString()} 行
          </span>
        </Space>
      }
      footer={[
        <Space key="f" style={{ justifyContent: 'space-between', width: '100%' }}>
          <Space key="l">
            <span style={{ fontSize: 12, color: JNJ.textMuted }}>每页</span>
            <Select
              size="small"
              value={pageSize}
              onChange={setPageSize}
              style={{ width: 90 }}
              options={[200, 500, 1000, 2000, 5000].map((n) => ({ label: `${n} 行`, value: n }))}
            />
            <Tooltip title="重新读取">
              <a onClick={() => void load()} style={{ color: JNJ.text }}>
                <RefreshCw size={14} />
              </a>
            </Tooltip>
          </Space>
          <Space key="r">
            <Button icon={<FileDown size={14} />} disabled={exporting} onClick={() => void doExport(false)}>
              导出 .xlsm
            </Button>
            <Button icon={<FileDown size={14} />} disabled={exporting} onClick={() => void doExport(true)}>
              导出 .xlsx
            </Button>
            <Button type="primary" onClick={onClose} style={{ background: JNJ.red, borderColor: JNJ.red }}>
              关闭
            </Button>
          </Space>
        </Space>,
      ]}
    >
      <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
        <Input
          allowClear
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          prefix={<Search size={14} style={{ color: JNJ.textMuted }} />}
          placeholder="搜索索引列"
          style={{ width: 320 }}
        />
      </div>
      <Table<GridRow>
        size="small"
        bordered
        sticky
        rowKey="seq"
        loading={loading}
        columns={columns}
        dataSource={rows}
        scroll={{ x: 'max-content', y: 'calc(100vh - 340px)' }}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该周暂无数据" /> }}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: false,
          showTotal: (t, r) => `第 ${r[0]}-${r[1]} 条 / 共 ${t.toLocaleString()} 条`,
        }}
        onChange={(pagination, _f, sorter) => {
          const s = Array.isArray(sorter) ? sorter[0] : sorter;
          const key = String(s?.columnKey ?? '');
          setSort(key && s?.order ? { field: key, desc: s.order === 'descend' } : { field: 'seq', desc: false });
          setPage(pagination.current ?? 1);
        }}
      />
    </Modal>
  );
}
