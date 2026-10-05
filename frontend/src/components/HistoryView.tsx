import { useState } from 'react';
import { Modal, Input, Button, Space, Tag, Tooltip, Select } from 'antd';
import { Search, FileDown, RefreshCw } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { useTableQuery } from '../hooks/useTableQuery';
import { buildGridColumns } from '../lib/gridColumns';
import { GridTable } from './GridTable';
import type { ExportResult } from '../types';

export interface HistoryViewProps {
  weekCode: string | null;
  onClose: () => void;
  onExported: (msg: string) => void;
  /**
   * 导出交给 App 统一执行：这样状态栏的进度与完成标记对历史数据窗口同样生效
   * （这里曾经直接调 api.export，进度事件落在任务包装之外，无人认领）。
   */
  onExport: (mode: 'clean' | 'template') => Promise<ExportResult | null>;
  /**
   * 首次载入完成（拿到数据，或确认这一周没有数据）时回调一次。
   * App 用它收起点击后立刻盖上的全屏加载遮罩。
   */
  onReady?: () => void;
}

/**
 * The full table for one committed week. Sorts and filters are done by the
 * backend, but the page size is deliberately larger than the main grid because
 * the brief asks for "all data" here.
 */
export function HistoryView({ weekCode, onClose, onExported, onExport, onReady }: HistoryViewProps) {
  const q = useTableQuery({
    source: weekCode,
    defaultPageSize: 500,
    // 失败时保留原有行：这是这个窗口一直以来的行为。
    clearRowsOnError: false,
    onSettled: onReady,
  });
  const [exporting, setExporting] = useState(false);

  const { columns, tableWidth } = buildGridColumns({
    header: q.header,
    // 历史窗口始终按两行表头显示。
    headerDisplay: 'twoRow',
    sort: q.sort,
    indexTitle: (n) => `固定索引 第 ${n} 列`,
  });

  const doExport = async (mode: 'clean' | 'template') => {
    if (!weekCode) return;
    setExporting(true);
    try {
      const res = await onExport(mode);
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
          {q.header?.weekCode && (
            <>
              <Tag style={{ background: JNJ.red, color: '#fff', border: 'none' }}>
                {q.header.weekCode}
              </Tag>
              <span style={{ color: JNJ.text, fontWeight: 400, fontSize: 13 }}>{q.header.weekStart}</span>
            </>
          )}
          <span style={{ color: JNJ.textMuted, fontWeight: 400, fontSize: 12 }}>
            共 {q.total.toLocaleString()} 行
          </span>
        </Space>
      }
      footer={[
        <Space key="f" style={{ justifyContent: 'space-between', width: '100%' }}>
          <Space key="l">
            <span style={{ fontSize: 12, color: JNJ.textMuted }}>每页</span>
            <Select
              size="small"
              value={q.pageSize}
              onChange={q.changePageSize}
              style={{ width: 90 }}
              options={[200, 500, 1000, 2000, 5000].map((n) => ({ label: `${n} 行`, value: n }))}
            />
            <Tooltip title="重新读取">
              <a onClick={q.reload} style={{ color: JNJ.text }}>
                <RefreshCw size={14} />
              </a>
            </Tooltip>
          </Space>
          <Space key="r">
            <Button icon={<FileDown size={14} />} disabled={exporting} onClick={() => void doExport('clean')}>
              导出纯数据 (.xlsx)
            </Button>
            <Button icon={<FileDown size={14} />} disabled={exporting} onClick={() => void doExport('template')}>
              导出原文件格式 (.xlsm)
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
          value={q.search}
          onChange={(e) => q.setSearch(e.target.value)}
          prefix={<Search size={14} style={{ color: JNJ.textMuted }} />}
          placeholder="搜索索引列"
          style={{ width: 320 }}
        />
      </div>
      <GridTable
        rows={q.rows}
        columns={columns}
        loading={q.loading}
        emptyText="该周暂无数据"
        scrollX={tableWidth}
        scrollY="calc(100vh - 340px)"
        pagination={{
          current: q.page,
          pageSize: q.pageSize,
          total: q.total,
          showSizeChanger: false,
          showTotal: (t, r) => `第 ${r[0]}-${r[1]} 条 / 共 ${t.toLocaleString()} 条`,
        }}
        onChange={q.onTableChange}
      />
    </Modal>
  );
}
