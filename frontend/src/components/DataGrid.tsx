import { useLayoutEffect, useRef, useState } from 'react';
import { Input, Pagination, Space, Tag, Tooltip } from 'antd';
import { Search, RotateCw } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { useTableQuery } from '../hooks/useTableQuery';
import { buildGridColumns } from '../lib/gridColumns';
import { GridTable } from './GridTable';

export interface DataGridProps {
  /** "" shows the staging area; a week code shows that committed week. */
  source: string;
  headerDisplay: 'twoRow' | 'oneRow';
  pageSize: number;
  /** Bumped by the parent after an import or commit to force a reload. */
  reloadToken: number;
  /** The list's name: 数据清单 plus the state suffix, worded by the shell. */
  title: string;
  /** Shown when there are no rows; the shell words it from the data state. */
  emptyText?: string;
}

export function DataGrid({ source, headerDisplay, pageSize, reloadToken, title, emptyText }: DataGridProps) {
  const q = useTableQuery({
    source,
    defaultPageSize: pageSize,
    reloadToken,
    // Leaving the previous week's rows under a new week's header would be a lie.
    clearRowsOnError: true,
  });

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
  }, [q.header]);

  const { columns, tableWidth } = buildGridColumns({
    header: q.header,
    headerDisplay,
    sort: q.sort,
    indexTitle: (n) => `第 ${n} 列 · 固定索引`,
    // Where a row came from, so a value can be traced back to its workbook.
    cellTitle: (row) => (row.fileName ? `${row.fileName} · 源第 ${row.srcRow} 行` : undefined),
  });

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
        {q.header?.weekCode && (
          <Tag
            style={{
              background: 'rgba(255,255,255,0.22)',
              color: '#fff',
              border: 'none',
              margin: 0,
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {q.header.weekCode} · {q.header.weekStart}
          </Tag>
        )}
        <span style={{ marginLeft: 'auto', fontWeight: 400, opacity: 0.92, fontSize: 12 }}>
          共 {q.total.toLocaleString()} 行 · {q.header?.weeks?.length ?? 0} 个周列
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
          value={q.search}
          onChange={(e) => q.setSearch(e.target.value)}
          prefix={<Search size={14} style={{ color: JNJ.textMuted }} />}
          placeholder="搜索 P1..P5 / 物料 / 描述 / 目录 ..."
          style={{ width: 300 }}
        />
        <Space style={{ marginLeft: 'auto' }}>
          <Tooltip title="刷新">
            <a onClick={q.reload} style={{ color: JNJ.text }}>
              <RotateCw size={15} className={q.loading ? 'spin' : undefined} />
            </a>
          </Tooltip>
        </Space>
      </div>

      {/* One scrollbar only: the table body scrolls inside this box, while the
          pagination sits below it and never scrolls out of view. */}
      <div ref={scrollRef} style={{ flex: 1, minHeight: 0, overflow: 'hidden', background: JNJ.surface }}>
        <GridTable
          rows={q.rows}
          columns={columns}
          loading={q.loading}
          emptyText={emptyText ?? (source === '' ? '暂无临时数据，请使用工具栏的导入或添加' : '该周暂无数据')}
          scrollX={tableWidth}
          scrollY={bodyHeight > 0 ? bodyHeight : 'calc(100vh - 520px)'}
          onChange={q.onTableChange}
        />
      </div>

      {/* 第 0-0 条 / 共 0 条 is noise under an empty list, and its top border is one
          more line across empty space: with nothing to page through there is no
          footer at all. */}
      {q.rows.length > 0 && (
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
          current={q.page}
          pageSize={q.pageSize}
          total={q.total}
          showSizeChanger
          pageSizeOptions={[50, 100, 200, 500, 1000]}
          showTotal={(t, range) => `第 ${range[0]}-${range[1]} 条 / 共 ${t.toLocaleString()} 条`}
          onChange={(nextPage, nextSize) => {
            if (nextSize !== q.pageSize) q.changePageSize(nextSize);
            else q.setPage(nextPage);
          }}
        />
      </div>
      )}
    </div>
  );
}
