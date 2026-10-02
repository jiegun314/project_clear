import { useEffect, useState } from 'react';
import { Select, Button, Space, Typography, Empty, Tooltip } from 'antd';
import { History, Eye } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { ArchiveEntry } from '../types';

export interface HistoryPanelProps {
  /** Fixed minimum width, as required: the panel must not follow the window. */
  onView: (weekCode: string) => void;
  reloadToken: number;
}

/**
 * The lower-left module: pick a year and an ISO week, then open that week's
 * full table. Width is pinned so a wide data grid cannot squeeze it away.
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
        if (ys && ys.length) {
          setYear((prev) => (prev && ys.includes(prev) ? prev : ys[0]));
        }
      } catch {
        if (alive) {
          setYears([]);
          setArchive([]);
        }
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
    if (year == null) {
      setWeeks([]);
      return;
    }
    let alive = true;
    api
      .getWeeks(year)
      .then((ws) => {
        if (!alive) return;
        setWeeks(ws ?? []);
        setWeek((prev) => (prev && (ws ?? []).includes(prev) ? prev : (ws ?? [])[0] ?? null));
      })
      .catch(() => alive && setWeeks([]));
    return () => {
      alive = false;
    };
  }, [year]);

  const weekCode = year != null && week != null ? `${String(year % 100).padStart(2, '0')}${String(week).padStart(2, '0')}` : null;
  const known = weekCode ? archive.some((e) => e.weekCode === weekCode) : false;
  const entry = weekCode ? archive.find((e) => e.weekCode === weekCode) : undefined;

  return (
    <div
      style={{
        width: 300,
        minWidth: 300,
        maxWidth: 300,
        display: 'flex',
        flexDirection: 'column',
        background: JNJ.surface,
        borderRight: `1px solid ${JNJ.border}`,
        minHeight: 0,
      }}
    >
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
          gap: 8,
        }}
      >
        <History size={14} />
        <span>历史数据</span>
        <span style={{ marginLeft: 'auto', fontWeight: 400, opacity: 0.92, fontSize: 12 }}>
          {archive.length} 周
        </span>
      </div>

      <div style={{ padding: 10, display: 'flex', flexDirection: 'column', gap: 8, flex: '0 0 auto' }}>
        <Space.Compact style={{ width: '100%' }}>
          <Select
            style={{ width: '50%' }}
            placeholder="年份"
            value={year}
            onChange={setYear}
            options={years.map((y) => ({ label: `${y}`, value: y }))}
            loading={loading}
            notFoundContent={<Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />}
          />
          <Select
            style={{ width: '50%' }}
            placeholder="周"
            value={week}
            onChange={setWeek}
            options={weeks.map((w) => ({ label: `第 ${w} 周`, value: w }))}
            notFoundContent={<Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />}
          />
        </Space.Compact>
        <Button
          type="primary"
          icon={<Eye size={15} />}
          block
          disabled={!weekCode || !known}
          onClick={() => weekCode && onView(weekCode)}
          style={{ background: JNJ.red, borderColor: JNJ.red }}
        >
          查看
        </Button>
        {entry && (
          <Typography.Text style={{ fontSize: 12, color: JNJ.textMuted, lineHeight: 1.5 }}>
            {entry.weekCode} · {entry.weekStart}
            <br />
            {entry.rowCount.toLocaleString()} 行 / {entry.fileCount} 个文件
            <br />
            整合于 {entry.committedAt}
          </Typography.Text>
        )}
        {year != null && !known && (
          <Tooltip title="该周尚未整合入库">
            <Typography.Text style={{ fontSize: 12, color: JNJ.warning }}>
              该周暂无数据
            </Typography.Text>
          </Tooltip>
        )}
      </div>

      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', borderTop: `1px solid ${JNJ.divider}` }}>
        {archive.length === 0 ? (
          <div style={{ padding: 16 }}>
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未整合任何周数据" />
          </div>
        ) : (
          archive.map((e) => (
            <div
              key={e.weekCode}
              onClick={() => onView(e.weekCode)}
              style={{
                padding: '7px 12px',
                borderBottom: `1px solid ${JNJ.divider}`,
                cursor: 'pointer',
                background: e.weekCode === weekCode ? JNJ.redSoft : undefined,
              }}
            >
              <div style={{ fontWeight: 600, color: JNJ.ink, fontSize: 13 }}>
                {e.weekCode} <span style={{ fontWeight: 400, color: JNJ.textMuted, fontSize: 12 }}>{e.weekStart}</span>
              </div>
              <div style={{ fontSize: 12, color: JNJ.text }}>
                {e.year} 年第 {e.weekNo} 周 · {e.rowCount.toLocaleString()} 行
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
