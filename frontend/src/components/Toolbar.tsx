import { Tooltip, Button, Divider } from 'antd';
import {
  FolderInput,
  FilePlus2,
  Database,
  FileDown,
  Settings,
  Info,
  Power,
} from 'lucide-react';
import { JNJ } from '../theme/jnj';

export interface ToolbarProps {
  busy: boolean;
  hasStaging: boolean;
  hasArchive: boolean;
  onImport: () => void;
  onAdd: () => void;
  onCommit: () => void;
  onExport: () => void;
  onSettings: () => void;
  onAbout: () => void;
  onQuit: () => void;
}

/**
 * The toolbar is icon-only: every action reveals its name on hover, as the
 * brief requires. Import/add are the primary actions and carry the J&J red;
 * the rest stay neutral so the eye lands on the data actions first.
 */
export function Toolbar(p: ToolbarProps) {
  const actions: {
    key: string;
    icon: React.ReactNode;
    label: string;
    onClick: () => void;
    primary?: boolean;
    disabled?: boolean;
  }[] = [
    { key: 'import', icon: <FolderInput size={18} />, label: '导入文件夹中的所有源文件', onClick: p.onImport, primary: true },
    { key: 'add', icon: <FilePlus2 size={18} />, label: '添加单个源文件', onClick: p.onAdd, primary: true },
    { key: 'commit', icon: <Database size={18} />, label: '整合：将临时数据转为永久周数据表', onClick: p.onCommit, disabled: !p.hasStaging },
    { key: 'export', icon: <FileDown size={18} />, label: '导出：另存为 Excel', onClick: p.onExport, disabled: !p.hasStaging && !p.hasArchive },
    { key: 'settings', icon: <Settings size={18} />, label: '参数设定', onClick: p.onSettings },
    { key: 'about', icon: <Info size={18} />, label: '关于', onClick: p.onAbout },
    { key: 'quit', icon: <Power size={18} />, label: '退出程序', onClick: p.onQuit },
  ];

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
      {actions.map((a) => (
        <Tooltip key={a.key} title={a.label} mouseEnterDelay={0.15}>
          <Button
            type={a.primary ? 'primary' : 'default'}
            icon={a.icon}
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
      ))}
      {p.busy && (
        <span style={{ marginLeft: 12, color: JNJ.red, fontSize: 12 }}>处理中…</span>
      )}
    </div>
  );
}
