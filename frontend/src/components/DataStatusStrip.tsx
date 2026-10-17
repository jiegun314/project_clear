import { CircleCheck, Combine, Eraser, type LucideIcon } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import type { DataState } from '../hooks/useAppActions';

/**
 * What the data area is holding, in one line above the table.
 *
 * The table itself answers "what are the numbers"; this answers "which of the
 * two inputs am I looking at, and can 整合 run yet". Before 整合 the strip names
 * both sides and lets the user switch between them; the merged result only
 * exists afterwards.
 */
export function DataStatusStrip({
  state,
  onSelectDataset,
}: {
  state: DataState;
  onSelectDataset?: (dataset: 'global' | 'local') => void;
}) {
  const view = describe(state);
  if (!view) return null;
  const canSwitch = state.kind === 'staging' && state.hasGlobal && state.hasLocal && onSelectDataset;

  return (
    <div
      style={{
        flex: '0 0 auto',
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '4px 10px',
        marginBottom: 6,
        background: view.bg,
        border: `1px solid ${view.border}`,
        borderRadius: 4,
        fontSize: 12,
        color: view.fg,
        minHeight: 28,
      }}
    >
      <view.Icon size={14} style={{ flex: '0 0 auto', color: view.fg }} />
      <b style={{ fontWeight: 600 }}>{view.label}</b>
      {view.detail && <span style={{ color: JNJ.text }}>{view.detail}</span>}
      {view.note && <span style={{ color: JNJ.textMuted }}>· {view.note}</span>}
      {state.kind === 'staging' && state.replaced.length > 0 && (
        <span style={{ color: JNJ.textMuted }}>
          · 已覆盖 {state.replaced.length} 个同名文件（{state.replaced.join('、')}）
        </span>
      )}
      {canSwitch && (
        <span style={{ marginLeft: 'auto', display: 'inline-flex', gap: 4 }}>
          {(['global', 'local'] as const).map((d) => {
            const active = state.dataset === d;
            return (
              <button
                key={d}
                type="button"
                aria-pressed={active}
                aria-label={`查看 ${d === 'global' ? 'Global' : 'Local'} 数据`}
                onClick={() => onSelectDataset(d)}
                style={{
                  border: `1px solid ${active ? JNJ.red : JNJ.border}`,
                  background: active ? JNJ.red : JNJ.surface,
                  color: active ? '#fff' : JNJ.text,
                  borderRadius: 3,
                  fontSize: 11,
                  padding: '1px 8px',
                  cursor: 'pointer',
                  fontWeight: active ? 600 : 400,
                }}
              >
                {d === 'global' ? 'Global' : 'Local'}
              </button>
            );
          })}
        </span>
      )}
    </div>
  );
}

interface StripView {
  Icon: LucideIcon;
  label: string;
  detail: string;
  note?: string;
  bg: string;
  border: string;
  fg: string;
}

/** The words and colours for one state; null means "nothing worth a strip". */
function describe(state: DataState): StripView | null {
  switch (state.kind) {
    case 'staging': {
      const both = state.hasGlobal && state.hasLocal;
      const missingLocal = state.hasGlobal && !state.hasLocal;
      return {
        Icon: Combine,
        label: both ? '待整合' : '待补齐',
        detail: `周码 ${state.weekCode} · Global ${
          state.hasGlobal ? `${state.globalRows.toLocaleString()} 行` : '未导入'
        } · Local ${state.hasLocal ? `${state.localRows.toLocaleString()} 行` : '未导入'}`,
        note: both
          ? '两份数据已就绪，点整合生成最终结果'
          : missingLocal
            ? '还需要导入本地 MPS 文件'
            : '还需要导入 Global 数据',
        bg: both ? JNJ.successSoft : JNJ.warningSoft,
        border: both ? '#CBE6D5' : '#F0DCB8',
        fg: both ? JNJ.success : JNJ.warning,
      };
    }
    case 'committed':
      return {
        Icon: CircleCheck,
        label: '已整合',
        detail: `周码 ${state.weekCode}（${state.weekStart}）· ${state.rows.toLocaleString()} 行 · ${state.files} 个文件`,
        note: state.overwrote ? '已覆盖原有同期数据' : undefined,
        bg: JNJ.successSoft,
        border: '#CBE6D5',
        fg: JNJ.success,
      };
    case 'cleared':
      return {
        Icon: Eraser,
        label: '已清空',
        detail:
          state.rows > 0
            ? `周码 ${state.weekCode} · ${state.files} 个文件 · ${state.rows.toLocaleString()} 行`
            : '没有未整合的临时数据',
        note: '已整合入库的数据不受影响',
        bg: JNJ.fill,
        border: JNJ.border,
        fg: JNJ.text,
      };
    default:
      return null;
  }
}

/**
 * The list's own name, with what has happened to the data as a suffix: the list
 * is always 数据清单, and the suffix says which side the grid is showing.
 */
export function titleFor(state: DataState): string {
  switch (state.kind) {
    case 'staging':
      return `数据清单 - ${state.dataset === 'local' ? 'Local' : 'Global'}（待整合）`;
    case 'committed':
      return `数据清单 - 已整合（周码 ${state.weekCode}）`;
    case 'cleared':
      return '数据清单 - 已清空';
    default:
      return '数据清单';
  }
}

/** The wording the table shows when it has no rows, for the same data states. */
export function emptyTextFor(state: DataState): string {
  switch (state.kind) {
    case 'staging':
      return '这份数据还没有导入';
    case 'cleared':
      return '数据已清空，请使用工具栏的导入或添加';
    case 'committed':
      return '该周暂无数据';
    default:
      return '暂无数据，请使用工具栏的导入或添加';
  }
}
