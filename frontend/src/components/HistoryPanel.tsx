import { useEffect, useState } from 'react';
import { Select, Button, Typography, Empty, Tooltip, Tag } from 'antd';
import { History, Eye } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { ArchiveEntry } from '../types';
import { DatasetTag } from './DatasetTag';

export interface HistoryPanelProps {
  /** Fixed minimum width, as required: the panel must not follow the window. */
  onView: (datasetId: number, dataset: string) => void;
  reloadToken: number;
}

/**
 * The lower-left module: pick a year and an ISO week, then open that week's
 * integrated datasets. One week holds 整合 / Global / Local; each is a separate
 * dataset and opens on its own.
 */
export function HistoryPanel({ onView, reloadToken }: HistoryPanelProps) {
  const [years, setYears] = useState<number[]>([]);
  const [weeks, setWeeks] = useState<number[]>([]);
  const [year, setYear] = useState<number | null>(null);
  const [week, setWeek] = useState<number | null>(null);
  const [archive, setArchive] = useState<ArchiveEntry[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      setLoading(true);
      try {
        const [ys, list] = await Promise.all([api.getYears(), api.getArchive()]);
        if (!alive) return;
        setYears(ys ?? []);
        setArchive(list ?? []);
        if (ys && ys.length) setYear((prev) => prev ?? ys[0]);
        else setYear(null);
      } finally {
        if (alive) setLoading(false);
      }
    };
    void load();
    return () => {
      alive = false;
    };
  }, [reloadToken]);

  useEffect(() => {
    let alive = true;
    if (year == null) {
      setWeeks([]);
      setWeek(null);
      return;
    }
    void api
      .getWeeks(year)
      .then((ws) => {
        if (!alive) return;
        setWeeks(ws ?? []);
        setWeek((prev) => (prev && ws?.includes(prev) ? prev : ws?.[0] ?? null));
      })
      .catch(() => {
        if (alive) setWeeks([]);
      });
    return () => {
      alive = false;
    };
  }, [year, reloadToken]);

  const target = (() => {
    if (year == null || week == null) return undefined;
    const entries = archive.filter((e) => e.year === year && e.weekNo === week);
    return entries.find((e) => e.dataset === 'merged') ?? entries[0];
  })();
  const known = Boolean(target);

  return (
    <div
      style={{
        flex: '0 0 300px',
        width: 300,
        minWidth: 300,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        background: JNJ.surface,
        borderRight: `1px solid ${JNJ.border}`,
      }}
    >
      <div
        style={{
          flex: '0 0 auto',
          padding: '7px 12px',
          background: JNJ.red,
          color: '#fff',
          fontWeight: 600,
          fontSize: 13,
          letterSpacing: 0.5,
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <History size={14} />
        <span>历史数据</span>
        <span style={{ marginLeft: 'auto', fontWeight: 400, opacity: 0.92, fontSize: 12 }}>
          {new Set(archive.map((e) => e.weekCode)).size} 周
        </span>
      </div>

      {/* One row: year, week and 查看. */}
      <div style={{ padding: '8px 10px', display: 'flex', alignItems: 'center', gap: 6, flex: '0 0 auto' }}>
        <Select
          style={{ width: 86, flex: '0 0 86px' }}
          placeholder="年份"
          aria-label="年份"
          value={year}
          onChange={setYear}
          options={years.map((y) => ({ label: `${y}`, value: y }))}
          loading={loading}
          notFoundContent={<Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />}
        />
        <Select
          style={{ flex: 1, minWidth: 0 }}
          placeholder="周"
          aria-label="周"
          value={week}
          onChange={setWeek}
          options={weeks.map((w) => ({ label: `第 ${w} 周`, value: w }))}
          notFoundContent={<Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />}
        />
        <Button
          type="primary"
          icon={<Eye size={15} />}
          disabled={!known || !target}
          onClick={() => target && onView(target.datasetId, target.dataset)}
          style={{ background: JNJ.red, borderColor: JNJ.red, flex: '0 0 auto' }}
        >
          查看
        </Button>
      </div>
      {target?.expired && (
        <div style={{ padding: '0 10px 6px', display: 'flex', alignItems: 'center', gap: 6 }}>
          <Tag style={{ margin: 0, padding: '0 5px', fontSize: 10, lineHeight: '16px', color: JNJ.warning, borderColor: '#F0DCB8', background: JNJ.warningSoft }}>
            该周整合已过期
          </Tag>
          <span style={{ fontSize: 11, color: JNJ.warning }}>历史仍是上次整合版本</span>
        </div>
      )}
      {year != null && !known && (
        <div style={{ padding: '0 10px 6px' }}>
          <Tooltip title="该周尚未整合入库">
            <Typography.Text style={{ fontSize: 11, color: JNJ.warning }}>该周暂无数据</Typography.Text>
          </Tooltip>
        </div>
      )}

      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', borderTop: `1px solid ${JNJ.divider}` }}>
        {archive.length === 0 ? (
          <div style={{ padding: 16 }}>
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未完成任何周数据导入" />
          </div>
        ) : (
          archive.map((e) => (
            <div
              key={`${e.weekCode}/${e.dataset}`}
              onClick={() => onView(e.datasetId, e.dataset)}
              style={{
                padding: '7px 12px',
                borderBottom: `1px solid ${JNJ.divider}`,
                borderLeft: e.expired ? `3px solid ${JNJ.warning}` : '3px solid transparent',
                cursor: 'pointer',
                background: e.datasetId === target?.datasetId ? JNJ.redSoft : undefined,
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  flexWrap: 'wrap',
                  rowGap: 2,
                  gap: 6,
                  fontWeight: 600,
                  color: JNJ.ink,
                  fontSize: 13,
                }}
              >
                <DatasetTag dataset={e.dataset} />
                {e.expired && (
                  <Tag style={{ margin: 0, padding: '0 5px', fontSize: 10, lineHeight: '16px', color: JNJ.warning, borderColor: '#F0DCB8', background: JNJ.warningSoft }}>
                    已过期
                  </Tag>
                )}
                <span>{e.weekCode}</span>
                <span style={{ fontWeight: 400, color: JNJ.textMuted, fontSize: 12 }}>{e.weekStart}</span>
                <span style={{ fontWeight: 400, color: JNJ.textMuted, fontSize: 11 }}>v{e.version}</span>
              </div>
              <div style={{ fontSize: 12, color: JNJ.text }}>
                {e.year} 年第 {e.weekNo} 周 · {e.rowCount.toLocaleString()} 行 / {e.fileCount} 个文件
              </div>
              <div style={{ fontSize: 11, color: JNJ.textMuted }}>创建于 {e.committedAt}</div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
