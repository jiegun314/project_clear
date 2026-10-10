import { CircleCheck, Eraser, FileUp, type LucideIcon } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import type { DataState } from '../hooks/useAppActions';

/**
 * What the data area is holding, in one line above the table.
 *
 * The table itself answers "what are the numbers"; this answers "where did they
 * come from and have they been integrated yet", which is what 整合 and 清空
 * change. The three states are the ones the requirement names: 已导入（未整合）,
 * 已整合, 已清空.
 */
export function DataStatusStrip({ state }: { state: DataState }) {
  const view = describe(state);
  if (!view) return null;

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
      }}
    >
      <view.Icon size={14} style={{ flex: '0 0 auto', color: view.fg }} />
      <b style={{ fontWeight: 600 }}>{view.label}</b>
      {view.detail && <span style={{ color: JNJ.text }}>{view.detail}</span>}
      {view.note && <span style={{ color: JNJ.textMuted }}>· {view.note}</span>}
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
    case 'imported':
      return {
        Icon: FileUp,
        label: '已导入，未整合',
        detail: `周码 ${state.weekCode} · ${state.files} 个文件 · ${state.rows.toLocaleString()} 行`,
        note: state.replaced.length
          ? `已覆盖 ${state.replaced.length} 个同名文件（${state.replaced.join('、')}）`
          : '点工具栏的整合入库才会写入历史数据',
        bg: JNJ.warningSoft,
        border: '#F0DCB8',
        fg: JNJ.warning,
      };
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
        detail: state.rows > 0
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
 * is always 数据清单, and the suffix says where the numbers came from.
 */
export function titleFor(state: DataState): string {
  switch (state.kind) {
    case 'imported':
      return '数据清单 - 已导入';
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
    case 'imported':
      return '已导入，但这一周没有命中 LOC 筛选的数据';
    case 'cleared':
      return '数据已清空，请使用工具栏的导入或添加';
    case 'committed':
      return '该周暂无数据';
    default:
      return '暂无数据，请使用工具栏的导入或添加';
  }
}
