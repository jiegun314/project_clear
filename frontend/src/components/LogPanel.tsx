import { useEffect, useMemo, useRef, useState } from 'react';
import { Segmented, Button, Tooltip, Empty } from 'antd';
import { ScrollText, Eraser } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api, onEvent } from '../services/api';
import type { LogEntry } from '../types';

const LEVEL_COLOR: Record<string, string> = {
  info: JNJ.text,
  success: JNJ.success,
  warn: JNJ.warning,
  error: JNJ.danger,
};

const LEVEL_TAG: Record<string, string> = {
  info: '信息',
  success: '成功',
  warn: '警告',
  error: '错误',
};

/** The lower-right module: a live feed of everything the backend reported. */
export function LogPanel() {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [level, setLevel] = useState<'all' | 'info' | 'success' | 'warn' | 'error'>('all');
  const [stick, setStick] = useState(true);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let alive = true;
    api
      .getLogs(500)
      .then((e) => alive && setEntries((e ?? []).slice().reverse()))
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  // Live tail: the backend emits every line it writes.
  useEffect(() => {
    const off = onEvent<LogEntry>('log:entry', (e) => {
      if (!e) return;
      setEntries((prev) => [e, ...prev].slice(0, 2000));
    });
    return off;
  }, []);

  const filtered = useMemo(
    () => (level === 'all' ? entries : entries.filter((e) => e.level === level)),
    [entries, level],
  );

  useEffect(() => {
    if (stick && boxRef.current) boxRef.current.scrollTop = 0;
  }, [filtered.length, stick]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0, background: JNJ.surface }}>
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
        <ScrollText size={14} />
        <span>运行日志</span>
        <span style={{ marginLeft: 'auto', fontWeight: 400, opacity: 0.92, fontSize: 12 }}>
          {filtered.length} 条
        </span>
      </div>

      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '6px 10px',
          borderBottom: `1px solid ${JNJ.divider}`,
        }}
      >
        <Segmented
          size="small"
          value={level}
          onChange={(v) => setLevel(v as typeof level)}
          options={[
            { label: '全部', value: 'all' },
            { label: '信息', value: 'info' },
            { label: '成功', value: 'success' },
            { label: '警告', value: 'warn' },
            { label: '错误', value: 'error' },
          ]}
        />
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 10, alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: JNJ.textMuted }}>新日志置顶</span>
          <Tooltip title="清空日志">
            <a
              onClick={() => {
                setEntries([]);
                void api.clearLogs();
              }}
              style={{ color: JNJ.text }}
            >
              <Eraser size={14} />
            </a>
          </Tooltip>
        </div>
      </div>

      <div
        ref={boxRef}
        onScroll={(e) => {
          const t = e.currentTarget;
          setStick(t.scrollTop < 8);
        }}
        style={{ flex: 1, minHeight: 0, overflowY: 'auto', fontSize: 12, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}
      >
        {filtered.length === 0 ? (
          <div style={{ padding: 16 }}>
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无日志" />
          </div>
        ) : (
          filtered.map((e) => (
            <div
              key={`${e.seq}-${e.time}`}
              style={{
                display: 'flex',
                gap: 8,
                padding: '3px 10px',
                borderBottom: `1px solid ${JNJ.divider}`,
                color: LEVEL_COLOR[e.level] ?? JNJ.text,
              }}
            >
              <span style={{ color: JNJ.textMuted, flex: '0 0 auto' }}>{e.time}</span>
              <span
                style={{
                  flex: '0 0 auto',
                  fontWeight: 600,
                  color: LEVEL_COLOR[e.level] ?? JNJ.text,
                }}
              >
                [{LEVEL_TAG[e.level] ?? e.level}]
              </span>
              <span style={{ color: JNJ.textMuted, flex: '0 0 auto' }}>{e.source}</span>
              <span style={{ flex: 1, wordBreak: 'break-word', whiteSpace: 'pre-wrap' }}>{e.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
