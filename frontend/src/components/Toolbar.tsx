import { Tooltip, Button, Divider } from 'antd';
import {
  FolderInput,
  FilePlus2,
  FileSpreadsheet,
  // A database with the arrow pointing down into it: 整合 writes the merged
  // rows into the permanent weekly tables.
  DatabaseArrowDown,
  Trash2,
  FileDown,
  Files,
  Settings,
  Info,
  Power,
} from 'lucide-react';
import { JNJ } from '../theme/jnj';
import type { StagingFilesView } from '../types';

export interface ToolbarProps {
  busy: boolean;
  /** A workbench is open (it may hold one or both inputs). */
  hasStaging: boolean;
  /** Which inputs that workbench holds; 整合 needs both. */
  hasGlobal: boolean;
  hasLocal: boolean;
  hasArchive: boolean;
  /** The files behind the 已导入文件 button; null until the first load. */
  stagedFiles: StagingFilesView | null;
  onImport: () => void;
  onAdd: () => void;
  onImportLocal: () => void;
  onCommit: () => void;
  onClear: () => void;
  onExport: () => void;
  onViewStagedFiles: () => void;
  onSettings: () => void;
  onAbout: () => void;
  onQuit: () => void;
}

/**
 * The toolbar is icon-only: every action reveals its name on hover, as the
 * brief requires.
 *
 * The data actions are grouped by what they do, left to right:
 *
 *	Global 数据  导入文件夹 / 添加源文件
 *	Local 数据   导入本地 MPS
 *	合并与产出   整合 / 导出
 *	清单与重来   已导入文件 + 计数 / 清空
 *
 * The two inputs are peers — either may be imported first — so they sit in
 * separate groups rather than in one "import" run. The grouping is drawn with
 * vertical dividers only: the toolbar stays one row tall.
 */
export function Toolbar(p: ToolbarProps) {
  type Action = {
    key: string;
    icon: React.ReactNode;
    label: string;
    onClick: () => void;
    primary?: boolean;
    danger?: boolean;
    disabled?: boolean;
  };

  const globalActions: Action[] = [
    {
      key: 'import',
      icon: <FolderInput size={18} />,
      label: '导入 Global 文件夹：读取本周 MPS 导出的全部源文件',
      onClick: p.onImport,
      primary: true,
    },
    {
      key: 'add',
      icon: <FilePlus2 size={18} />,
      label: '添加 Global 源文件：把漏掉的文件补进当前清单',
      onClick: p.onAdd,
      primary: true,
    },
  ];

  const localActions: Action[] = [
    {
      key: 'local',
      icon: <FileSpreadsheet size={18} />,
      label: '导入本地 MPS 文件（Month End）：检查格式并写入 Local 数据',
      onClick: p.onImportLocal,
    },
  ];

  const mergeActions: Action[] = [
    {
      key: 'commit',
      icon: <DatabaseArrowDown size={18} />,
      label: '整合：合并 Global 与 Local，并写入永久周数据表',
      onClick: p.onCommit,
      disabled: !p.hasStaging || !p.hasGlobal || !p.hasLocal,
    },
    {
      key: 'export',
      icon: <FileDown size={18} />,
      label: '导出：整合后生成 MPS / Local / Global 三表文件',
      onClick: p.onExport,
      // Before 整合 there is no merged result to hand on, and while a workbench
      // is open the toolbar follows it rather than an older week.
      disabled: p.hasStaging || !p.hasArchive,
    },
  ];

  const appActions: Action[] = [
    { key: 'settings', icon: <Settings size={18} />, label: '参数设定', onClick: p.onSettings },
    { key: 'about', icon: <Info size={18} />, label: '关于', onClick: p.onAbout },
    { key: 'quit', icon: <Power size={18} />, label: '退出程序', onClick: p.onQuit },
  ];

  const renderActions = (list: Action[]) =>
    list.map((a) => (
      <Tooltip key={a.key} title={a.label} mouseEnterDelay={0.15}>
        <Button
          type={a.primary ? 'primary' : 'default'}
          icon={a.icon}
          danger={a.danger}
          disabled={p.busy || a.disabled}
          onClick={a.onClick}
          aria-label={a.label}
          style={
            a.primary
              ? { background: JNJ.red, borderColor: JNJ.red }
              : { color: JNJ.text }
          }
        />
      </Tooltip>
    ));

  const divider = <Divider orientation="vertical" style={{ height: 22, margin: '0 6px' }} />;

  const files = p.stagedFiles;
  const countLabel = p.hasStaging
    ? `${(files?.fileCount ?? 0).toLocaleString()} 个文件 · 待整合`
    : `${(files?.fileCount ?? 0).toLocaleString()} 个文件 · ${(files?.rowCount ?? 0).toLocaleString()} 行`;
  const failed = files?.failedCount ?? 0;

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 4,
        padding: '8px 14px',
        background: JNJ.surface,
        borderBottom: `1px solid ${JNJ.border}`,
        flex: '0 0 auto',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginRight: 8 }}>
        {/* The application's own macOS icon, cropped to the artwork and
            supplied at 2x. The rounded corners live in the PNG, so no CSS
            radius here -- adding one would double-round and clip it. */}
        <img src="/clear-mark.png" alt="CLEAR" style={{ width: 26, height: 26, display: 'block' }} />
        <span style={{ fontWeight: 600, color: JNJ.ink, letterSpacing: 1 }}>CLEAR</span>
        <span style={{ color: JNJ.textMuted, fontSize: 12 }}>MPS 数据整合平台</span>
      </div>
      {divider}
      {renderActions(globalActions)}
      {divider}
      {renderActions(localActions)}
      {divider}
      {renderActions(mergeActions)}
      {divider}
      {/* The workbench's file list as one small module: a section title, the
          count as the clickable way into the list, and 清空 next to it. Grouping
          them keeps the three related things from reading as separate toolbar
          actions; the height stays the one row of buttons. */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          height: 32,
          padding: '0 4px 0 10px',
          background: JNJ.surfaceAlt,
          border: `1px solid ${JNJ.border}`,
          borderRadius: 4,
          whiteSpace: 'nowrap',
        }}
      >
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 5,
            color: JNJ.textMuted,
            fontSize: 12,
          }}
        >
          <Files size={14} />
          已导入文件
        </span>
        <button
          type="button"
          aria-label="查看已导入文件"
          title="查看文件清单"
          disabled={p.busy}
          onClick={p.onViewStagedFiles}
          style={{
            border: 'none',
            background: 'transparent',
            padding: 0,
            color: JNJ.ink,
            fontSize: 12,
            fontWeight: 600,
            fontVariantNumeric: 'tabular-nums',
            cursor: p.busy ? 'default' : 'pointer',
            textDecoration: 'underline',
            textDecorationStyle: 'dotted',
            textUnderlineOffset: 3,
          }}
        >
          {countLabel}
        </button>
        {failed > 0 && (
          <span style={{ color: JNJ.danger, fontSize: 11 }}>· {failed} 个失败</span>
        )}
        <span style={{ width: 1, height: 16, background: JNJ.border }} aria-hidden="true" />
        <Tooltip
          title="清空：删除未整合的 Global 与 Local 数据，并清空文件列表；已整合的不受影响"
          mouseEnterDelay={0.15}
        >
          <Button
            type="text"
            size="small"
            danger
            icon={<Trash2 size={15} />}
            onClick={p.onClear}
            disabled={p.busy || (!p.hasStaging && !(files?.fileCount ?? 0))}
            aria-label="清空：删除未整合的数据与文件列表"
            style={{ width: 26, height: 26, minWidth: 26 }}
          />
        </Tooltip>
      </div>
      {p.busy && <span style={{ marginLeft: 12, color: JNJ.red, fontSize: 12 }}>处理中…</span>}
      {/* marginLeft:auto pushes the app-level actions to the right edge. */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginLeft: 'auto' }}>
        {renderActions(appActions)}
      </div>
    </div>
  );
}
