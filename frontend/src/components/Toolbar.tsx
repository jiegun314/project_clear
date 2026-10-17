import { Tooltip, Button, Divider } from 'antd';
import {
  FolderInput,
  FilePlus2,
  FileSpreadsheet,
  Check,
  DatabaseArrowDown,
  Trash2,
  FileDown,
  Files,
  Settings,
  Info,
  Power,
} from 'lucide-react';
import { JNJ } from '../theme/jnj';

export interface ToolbarProps {
  busy: boolean;
  hasDraft: boolean;
  hasGlobal: boolean;
  hasLocal: boolean;
  canIntegrate: boolean;
  hasIntegration: boolean;
  integrationExpired: boolean;
  /** "14 个文件 · 导入中" and friends; the 已导入文件 module's detail line. */
  countLabel: string;
  failedCount: number;
  onImport: () => void;
  onAdd: () => void;
  onFinish: () => void;
  onImportLocal: () => void;
  onCommit: () => void;
  onClear: () => void;
  onExport: () => void;
  onViewFiles: () => void;
  onSettings: () => void;
  onAbout: () => void;
  onQuit: () => void;
}

/**
 * The toolbar is icon-only: every action reveals its name on hover.
 *
 * The data actions are grouped by what they do, left to right:
 *
 *	Global 数据  导入文件夹 / 添加源文件 / 完成导入
 *	Local 数据   导入本地 MPS
 *	合并与产出   整合 / 导出
 *	清单        已导入文件 + 计数 / 丢弃草稿
 *
 * The two inputs are peers, but a Global import is a draft until 完成导入 is
 * pressed; that button is the explicit end signal. The grouping is drawn with
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
      label: '导入 Global 文件夹：读取本周 MPS 导出的全部源文件（生成草稿）',
      onClick: p.onImport,
      primary: true,
    },
    {
      key: 'add',
      icon: <FilePlus2 size={18} />,
      label: '添加 Global 源文件：把漏掉的文件补进当前导入',
      onClick: p.onAdd,
      primary: true,
    },
    {
      key: 'finish',
      icon: <Check size={18} />,
      label: '完成 Global 导入：把当前草稿定稿为可整合的数据集',
      onClick: p.onFinish,
      disabled: !p.hasDraft,
    },
  ];

  const localActions: Action[] = [
    {
      key: 'local',
      icon: <FileSpreadsheet size={18} />,
      label: '导入本地 MPS 文件（Month End）：检查格式并写入 Local 数据集',
      onClick: p.onImportLocal,
    },
  ];

  const mergeActions: Action[] = [
    {
      key: 'commit',
      icon: <DatabaseArrowDown size={18} />,
      label: '整合：合并已完成的 Global 与 Local，写入永久整合记录',
      onClick: p.onCommit,
      disabled: !p.canIntegrate,
    },
    {
      key: 'export',
      icon: <FileDown size={18} />,
      label: p.integrationExpired
        ? '导出：生成三表文件（当前整合已过期，导出的是上次整合结果）'
        : '导出：生成 MPS / Local / Global 三表文件',
      onClick: p.onExport,
      disabled: !p.hasIntegration,
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
          style={a.primary ? { background: JNJ.red, borderColor: JNJ.red } : { color: JNJ.text }}
        />
      </Tooltip>
    ));

  const divider = <Divider orientation="vertical" style={{ height: 22, margin: '0 6px' }} />;

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
        <img src="/clear-mark.png" alt="CLEAR" style={{ width: 26, height: 26, display: 'block' }} />
        <span style={{ fontWeight: 600, color: JNJ.ink, letterSpacing: 1 }}>CLEAR</span>
        <span style={{ color: JNJ.textMuted, fontSize: 12 }}>MPS 数据整合平台</span>
      </div>
      {divider}
      {/* The Global import module: the three actions and the workbook list it
          produces. Local is deliberately outside it — one file is the whole
          Local dataset, so its information lives in the status strip. */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
        {renderActions(globalActions)}
        {(p.hasDraft || p.hasGlobal) && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              height: 32,
              marginLeft: 4,
              padding: '0 4px 0 10px',
              background: JNJ.surfaceAlt,
              border: `1px solid ${JNJ.border}`,
              borderRadius: 4,
              whiteSpace: 'nowrap',
            }}
          >
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, color: JNJ.textMuted, fontSize: 12 }}>
              <Files size={14} />
              已导入文件
            </span>
            <button
              type="button"
              aria-label="查看 Global 已导入文件"
              title="查看 Global 文件清单"
              disabled={p.busy}
              onClick={p.onViewFiles}
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
              {p.countLabel}
            </button>
            {p.failedCount > 0 && <span style={{ color: JNJ.danger, fontSize: 11 }}>· {p.failedCount} 个失败</span>}
            <span style={{ width: 1, height: 16, background: JNJ.border }} aria-hidden="true" />
            <Tooltip
              title="清空：丢弃未完成的 Global 导入；已完成的导入与整合记录不受影响"
              mouseEnterDelay={0.15}
            >
              <Button
                type="text"
                size="small"
                danger
                icon={<Trash2 size={15} />}
                onClick={p.onClear}
                disabled={p.busy || !p.hasDraft}
                aria-label="清空：丢弃未完成的 Global 导入"
                style={{ width: 26, height: 26, minWidth: 26 }}
              />
            </Tooltip>
          </div>
        )}
      </div>
      {divider}
      {renderActions(localActions)}
      {divider}
      {renderActions(mergeActions)}
      {divider}
      {p.busy && <span style={{ marginLeft: 12, color: JNJ.red, fontSize: 12 }}>处理中…</span>}
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginLeft: 'auto' }}>
        {renderActions(appActions)}
      </div>
    </div>
  );
}
