import { Tooltip, Button, Divider } from 'antd';
import {
  FolderInput,
  FilePlus2,
  // A database with the arrow pointing down into it: 整合 writes the staged
  // rows into the permanent weekly table.
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
  hasStaging: boolean;
  hasArchive: boolean;
  /** The files behind the 已导入文件 button; null until the first load. */
  stagedFiles: StagingFilesView | null;
  onImport: () => void;
  onAdd: () => void;
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
 * brief requires. Import/add are the primary actions and carry the J&J red;
 * 清空 is the only destructive one and is outlined in red, the rest stay
 * neutral so the eye lands on the data actions first.
 *
 * The data actions stay next to the logo; 参数设定 / 关于 / 退出 are pushed to
 * the right edge, away from the ones a planner uses every week.
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

  const dataActions: Action[] = [
    { key: 'import', icon: <FolderInput size={18} />, label: '导入文件夹中的所有源文件', onClick: p.onImport, primary: true },
    { key: 'add', icon: <FilePlus2 size={18} />, label: '添加单个源文件', onClick: p.onAdd, primary: true },
    { key: 'commit', icon: <DatabaseArrowDown size={18} />, label: '整合：将临时数据写入永久周数据表', onClick: p.onCommit, disabled: !p.hasStaging },
    {
      key: 'clear',
      icon: <Trash2 size={18} />,
      label: '清空：删除所有已导入但未整合的临时数据，并清空已导入文件列表',
      onClick: p.onClear,
      danger: true,
      // After 整合 the temporary rows are gone, but the file list stays on
      // screen as the record of that import until it is cleared here.
      disabled: !p.hasStaging && !(p.stagedFiles?.fileCount ?? 0),
    },
    { key: 'export', icon: <FileDown size={18} />, label: '导出：纯数据或原文件格式（仅 MPS 页）', onClick: p.onExport, disabled: !p.hasStaging && !p.hasArchive },
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
      <Divider type="vertical" style={{ height: 22, margin: '0 6px' }} />
      {renderActions(dataActions)}
      <Divider type="vertical" style={{ height: 22, margin: '0 6px' }} />
      {/* The list behind 添加: which workbooks are in the integration list, and
          how many rows they brought in. It empties with 清空. */}
      <Tooltip title="查看整合清单里的文件" mouseEnterDelay={0.15}>
        <Button
          icon={<Files size={18} />}
          onClick={p.onViewStagedFiles}
          disabled={p.busy}
          aria-label="查看已导入文件"
          style={{ color: JNJ.text }}
        >
          已导入文件
        </Button>
      </Tooltip>
      <span style={{ fontSize: 12, color: JNJ.textMuted, whiteSpace: 'nowrap' }}>
        {(p.stagedFiles?.fileCount ?? 0).toLocaleString()} 个文件 ·{' '}
        {(p.stagedFiles?.rowCount ?? 0).toLocaleString()} 行
        {(p.stagedFiles?.failedCount ?? 0) > 0 ? ` · ${p.stagedFiles?.failedCount} 个失败` : ''}
      </span>
      {p.busy && (
        <span style={{ marginLeft: 12, color: JNJ.red, fontSize: 12 }}>处理中…</span>
      )}
      {/* marginLeft:auto pushes the app-level actions to the right edge. */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginLeft: 'auto' }}>
        {renderActions(appActions)}
      </div>
    </div>
  );
}
