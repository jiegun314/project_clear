import { Combine, Eraser, TriangleAlert } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import type { DataState, SelectedKind } from '../hooks/useAppActions';

/**
 * The workbench in one line: the Global import (draft or completed), the Local
 * import, and the week's integration — clickable where a dataset exists, so the
 * grid can switch between them. A newer import marks the integration expired.
 */
export function DataStatusStrip({
  state,
  onSelectDataset,
  onDiscardWeek,
}: {
  state: DataState;
  onSelectDataset?: (datasetId: number) => void;
  onDiscardWeek?: () => void;
}) {
  if (state.kind === 'none') return null;
  if (state.kind === 'cleared') {
    return (
      <Strip bg={JNJ.fill} border={JNJ.border} fg={JNJ.text}>
        <Eraser size={14} />
        <b>已清空</b>
        <span>
          {state.rows > 0
            ? `周码 ${state.week} · ${state.files} 个文件 · ${state.rows.toLocaleString()} 行`
            : '没有未整合的数据'}
        </span>
        <span style={{ color: JNJ.textMuted }}>· 已整合入库的数据不受影响</span>
      </Strip>
    );
  }

  const s = state.status;
  const chips: { key: SelectedKind; id: number; label: string; detail: string; enabled: boolean }[] = [
    {
      key: 'draft',
      id: s.draftId,
      label: s.hasDraft ? 'Global 导入中' : 'Global',
      detail: s.hasDraft
        ? `${s.draftFiles} 文件 · ${s.draftRows.toLocaleString()} 行`
        : s.hasGlobal
          ? `v${s.globalVersion} · ${s.globalRows.toLocaleString()} 行`
          : '未导入',
      enabled: s.hasDraft || s.hasGlobal,
    },
    {
      key: 'local',
      id: s.localId,
      label: 'Local',
      detail: s.hasLocal ? `v${s.localVersion} · ${s.localRows.toLocaleString()} 行` : '未导入',
      enabled: s.hasLocal,
    },
    {
      key: 'merged',
      id: s.integrationId,
      label: '整合',
      detail: s.hasIntegration
        ? s.integrationExpired
          ? '已过期'
          : `${s.integrationRows.toLocaleString()} 行`
        : '未整合',
      enabled: s.hasIntegration,
    },
  ];
  // The draft and the completed Global are two states of the same side; only one
  // chip is shown, and it targets whichever is current.
  if (s.hasDraft) chips[0].id = s.draftId;
  else chips[0].id = s.globalId;

  const expired = s.integrationExpired;
  const bg = expired || s.hasDraft || !s.hasGlobal || !s.hasLocal ? JNJ.warningSoft : JNJ.successSoft;
  const border = expired || s.hasDraft || !s.hasGlobal || !s.hasLocal ? '#F0DCB8' : '#CBE6D5';
  const fg = expired || s.hasDraft || !s.hasGlobal || !s.hasLocal ? JNJ.warning : JNJ.success;
  const note = (() => {
    if (s.hasDraft) return '点工具栏「完成 Global 导入」后这份数据才可整合';
    if (expired) return '有新导入，整合已过期，请重新整合';
    if (!s.hasGlobal) return '还需要导入 Global 数据';
    if (!s.hasLocal) return '还需要导入本地 MPS 文件';
    if (!s.hasIntegration) return '两份数据已就绪，可以整合';
    return '整合完成';
  })();

  return (
    <Strip bg={bg} border={border} fg={fg}>
      {expired ? <TriangleAlert size={14} /> : <Combine size={14} />}
      <b>{expired ? '整合已过期' : s.hasDraft ? '待完成导入' : '本周数据'}</b>
      {chips.map((c) => {
        const active = state.selected === c.id && c.enabled;
        return (
          <button
            key={c.key}
            type="button"
            disabled={!c.enabled || !onSelectDataset}
            onClick={() => c.enabled && onSelectDataset?.(c.id)}
            aria-label={`查看 ${c.label} 数据`}
            aria-pressed={active}
            title={c.key === 'local' && s.localFileName ? `Local 文件：${s.localFileName}` : undefined}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 5,
              border: `1px solid ${active ? JNJ.red : JNJ.border}`,
              background: active ? JNJ.red : JNJ.surface,
              color: active ? '#fff' : c.enabled ? JNJ.ink : JNJ.textMuted,
              borderRadius: 3,
              padding: '1px 7px',
              fontSize: 12,
              cursor: c.enabled ? 'pointer' : 'default',
            }}
          >
            <span style={{ fontWeight: 600 }}>{c.label}</span>
            <span style={{ opacity: 0.85 }}>{c.detail}</span>
          </button>
        );
      })}
      {s.hasLocal && s.localFileName && (
        <span
          title={`Local 文件：${s.localFileName}`}
          style={{
            color: JNJ.textMuted,
            maxWidth: 260,
            minWidth: 0,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            flexShrink: 1,
          }}
        >
          · Local 文件：{s.localFileName}
        </span>
      )}
      <span
        style={{
          color: JNJ.textMuted,
          minWidth: 0,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          flexShrink: 1,
        }}
      >
        · {note}
      </span>
      {s.canDiscardWeek && onDiscardWeek && (
        <button
          type="button"
          onClick={onDiscardWeek}
          style={{
            marginLeft: 'auto',
            border: 'none',
            background: 'transparent',
            padding: 0,
            color: JNJ.red,
            fontSize: 12,
            textDecoration: 'underline',
            cursor: 'pointer',
          }}
        >
          丢弃本周导入
        </button>
      )}
    </Strip>
  );
}

function Strip({
  children,
  bg,
  border,
  fg,
}: {
  children: React.ReactNode;
  bg: string;
  border: string;
  fg: string;
}) {
  return (
    <div
      style={{
        flex: '0 0 auto',
        display: 'flex',
        alignItems: 'center',
        flexWrap: 'nowrap',
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        gap: 6,
        padding: '4px 10px',
        marginBottom: 6,
        background: bg,
        border: `1px solid ${border}`,
        borderRadius: 4,
        fontSize: 12,
        color: fg,
        minHeight: 28,
      }}
    >
      {children}
    </div>
  );
}

/** The list's own name, with the selected side as the suffix. */
export function titleFor(state: DataState): string {
  if (state.kind === 'workspace') {
    const s = state.status;
    const kind = state.selectedKind;
    if (kind === 'draft') return '数据清单 - Global（导入中）';
    if (kind === 'local') return `数据清单 - Local v${s.localVersion}`;
    if (kind === 'global') return `数据清单 - Global v${s.globalVersion}`;
    return `数据清单 - 已整合（周码 ${s.integrationWeek || s.activeWeek}）`;
  }
  if (state.kind === 'cleared') return '数据清单 - 已清空';
  return '数据清单';
}

/** What the table says when it has no rows. */
export function emptyTextFor(state: DataState): string {
  switch (state.kind) {
    case 'workspace':
      return '这份数据还没有导入';
    case 'cleared':
      return '数据已清空，请使用工具栏的导入或添加';
    default:
      return '暂无数据，请使用工具栏的导入或添加';
  }
}
