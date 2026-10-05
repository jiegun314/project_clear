import { Spin, Tooltip } from 'antd';
import { CircleCheck } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { progressText, type TaskProgress } from '../lib/taskProgress';
import type { Status } from '../types';

/**
 * The bar along the bottom: what the backend currently holds, plus the progress
 * of whatever task is running (or the green tick that says it finished).
 */
export function StatusBar({
  status,
  progress,
}: {
  status: Status | null;
  progress: TaskProgress | null;
}) {
  const items: { label: string; value: string; tip?: string }[] = [
    { label: '临时数据', value: status?.hasStaging ? `${status.weekCode} · ${status.stagedRows.toLocaleString()} 行` : '无' },
    {
      label: '已整合',
      value: `${status?.archivedWeeks ?? 0} 周 · ${(status?.archivedRows ?? 0).toLocaleString()} 行`,
    },
    { label: '读取列数', value: String(status?.readColumns ?? '-') },
    { label: 'LOC 筛选', value: status?.locFilter ?? '-' },
    { label: '每页行数', value: String(status?.pageSize ?? '-') },
    { label: '导出模式', value: status?.exportMode === 'clean' ? '纯数据' : '原文件格式' },
  ];
  return (
    <div
      style={{
        flex: '0 0 auto',
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        padding: '5px 14px',
        background: JNJ.surface,
        borderTop: `1px solid ${JNJ.border}`,
        fontSize: 12,
        color: JNJ.text,
      }}
    >
      {progress &&
        (progress.finished ? (
          // 任务完成后留在状态栏的是绿色对勾，而不是红色转圈。
          <span
            style={{
              color: JNJ.success,
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              fontWeight: 600,
            }}
          >
            <CircleCheck size={14} />
            {progressText(progress)}
          </span>
        ) : (
          <span style={{ color: JNJ.red, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            <Spin size="small" />
            {progressText(progress)}
          </span>
        ))}
      {items.map((i) => (
        <Tooltip key={i.label} title={i.tip ?? `${i.label}：${i.value}`}>
          <span>
            <span style={{ color: JNJ.textMuted }}>{i.label} </span>
            <span style={{ fontWeight: 600, color: JNJ.ink }}>{i.value}</span>
          </span>
        </Tooltip>
      ))}
      {status?.database && (
        <Tooltip title={`数据库：${status.database}`}>
          <span style={{ marginLeft: 'auto', color: JNJ.textMuted, fontSize: 11 }}>本地数据库</span>
        </Tooltip>
      )}
    </div>
  );
}
