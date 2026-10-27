import { useEffect, useRef, useState } from 'react';
import { Select, Button, Empty, Tag, Tooltip } from 'antd';
import { History, ArrowRight } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { ArchiveEntry } from '../types';
import { DatasetTag } from './DatasetTag';

export interface HistoryPanelProps {
  /** Fixed minimum width, as required: the panel must not follow the window. */
  onView: (datasetId: number, dataset: string) => void;
  /** Loads the selected week back into the workbench (not the read-only view). */
  onActivate: (weekCode: string) => void;
  reloadToken: number;
  /**
   * The week the current workbench is on. When a new import completes, the
   * history selectors should follow it instead of staying on the previously
   * viewed week; an empty value (fresh launch) leaves the default selection.
   */
  activeWeek?: string;
}

/**
 * Week codes are YYWW. ActiveWeek can point at a week that has only a draft and
 * therefore no archive entry yet; deriving the year/week lets the filter follow
 * the workbench even then.
 */
function weekParts(code: string): { year: number; week: number } | null {
  if (!/^\d{4}$/.test(code)) return null;
  const year = 2000 + Number(code.slice(0, 2));
  const week = Number(code.slice(2));
  if (week < 1 || week > 53) return null;
  return { year, week };
}

/**
 * The lower-left module: pick a year and an ISO week, then open that week's
 * integrated datasets. One week holds 整合 / Global / Local; each is a separate
 * dataset and opens on its own.
 */
export function HistoryPanel({ onView, onActivate, reloadToken, activeWeek = '' }: HistoryPanelProps) {
  const [years, setYears] = useState<number[]>([]);
  const [weeks, setWeeks] = useState<number[]>([]);
  const [year, setYear] = useState<number | null>(null);
  const [week, setWeek] = useState<number | null>(null);
  const [archive, setArchive] = useState<ArchiveEntry[]>([]);
  const [loading, setLoading] = useState(false);
  // The week the user was last auto-focused on. A manual year/week change is
  // not fought by every refresh, but a new workbench week wins once.
  const lastFocus = useRef<string | null>(null);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      setLoading(true);
      try {
        const [ys, list] = await Promise.all([api.getYears(), api.getArchive()]);
        if (!alive) return;
        const rows = list ?? [];
        const focus = activeWeek ? rows.find((e) => e.weekCode === activeWeek) : undefined;
        const active = focus
          ? { year: focus.year, week: focus.weekNo }
          : activeWeek
            ? weekParts(activeWeek)
            : null;
        let nextYears = ys ?? [];
        if (active && !nextYears.includes(active.year)) {
          nextYears = [active.year, ...nextYears];
        }
        setYears(nextYears);
        setArchive(rows);
        if (!activeWeek) {
          lastFocus.current = null;
        }
        if (active && lastFocus.current !== activeWeek) {
          lastFocus.current = activeWeek;
          setYear(active.year);
          setWeek(active.week);
        } else {
          if (activeWeek && !active) lastFocus.current = null;
          if (nextYears.length) setYear((prev) => prev ?? nextYears[0]);
          else setYear(null);
        }
      } finally {
        if (alive) setLoading(false);
      }
    };
    void load();
    return () => {
      alive = false;
    };
  }, [reloadToken, activeWeek]);

  useEffect(() => {
    let alive = true;
    if (year == null) {
      setWeeks([]);
      setWeek(null);
      return;
    }
    const active = activeWeek ? weekParts(activeWeek) : null;
    void api
      .getWeeks(year)
      .then((ws) => {
        if (!alive) return;
        let options = ws ?? [];
        // A draft week has no ready archive entry yet; keep it selectable so the
        // filter can follow the workbench while the import is in progress.
        if (active && active.year === year && !options.includes(active.week)) {
          options = [...options, active.week];
        }
        setWeeks(options);
        setWeek((prev) => (prev && options.includes(prev) ? prev : options[0] ?? null));
      })
      .catch(() => {
        if (alive) setWeeks([]);
      });
    return () => {
      alive = false;
    };
  }, [year, reloadToken, activeWeek]);

  // The list is the selected week, not the whole archive. The archive itself
  // still drives the year/week options and the week count in the header.
  const visibleArchive =
    year == null || week == null
      ? []
      : archive.filter((e) => e.year === year && e.weekNo === week);
  const target =
    visibleArchive.find((e) => e.dataset === 'merged') ?? visibleArchive[0];
  const known = Boolean(target);
  const hasMerged = visibleArchive.some((e) => e.dataset === 'merged');

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

      {/* One row: year, week and the action that opens the week. Read-only
          history remains one click away on any dataset row below. */}
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
        <Tooltip
          title={
            known
              ? '把这一周重新载入主数据表；Global / Local 都已存在时可以直接整合'
              : '该周还没有已完成的导入'
          }
        >
          <Button
            type="primary"
            disabled={!known || !target}
            onClick={() => target && onActivate(target.weekCode)}
            style={{
              background: JNJ.red,
              borderColor: JNJ.red,
              flex: '0 0 auto',
              paddingInline: 10,
            }}
          >
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              打开
              <ArrowRight size={13} />
            </span>
          </Button>
        </Tooltip>
      </div>

      {known && !hasMerged && (
        <div style={{ padding: '0 10px 6px', display: 'flex', alignItems: 'center', gap: 6 }}>
          <Tag
            style={{
              margin: 0,
              padding: '0 5px',
              fontSize: 10,
              lineHeight: '16px',
              color: JNJ.warning,
              borderColor: '#F0DCB8',
              background: JNJ.warningSoft,
            }}
          >
            未整合
          </Tag>
          <span style={{ fontSize: 11, color: JNJ.warning }}>可继续处理</span>
        </div>
      )}

      {target?.expired && (
        <div style={{ padding: '0 10px 6px', display: 'flex', alignItems: 'center', gap: 6 }}>
          <Tag style={{ margin: 0, padding: '0 5px', fontSize: 10, lineHeight: '16px', color: JNJ.warning, borderColor: '#F0DCB8', background: JNJ.warningSoft }}>
            该周整合已过期
          </Tag>
          <span style={{ fontSize: 11, color: JNJ.warning }}>历史仍是上次整合版本</span>
        </div>
      )}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', borderTop: `1px solid ${JNJ.divider}` }}>
        {archive.length === 0 ? (
          <div style={{ padding: 16 }}>
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未完成任何周数据导入" />
          </div>
        ) : visibleArchive.length === 0 ? (
          <div style={{ padding: 16 }}>
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该周暂无数据" />
          </div>
        ) : (
          visibleArchive.map((e) => (
            <div
              key={`${e.weekCode}/${e.dataset}`}
              onClick={() => {
                setYear(e.year);
                setWeek(e.weekNo);
                onView(e.datasetId, e.dataset);
              }}
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
