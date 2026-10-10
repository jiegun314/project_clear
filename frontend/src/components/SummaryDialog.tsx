import { Modal } from 'antd';
import { JNJ } from '../theme/jnj';

export interface SummaryItem {
  label: string;
  value: string;
  /** Draws the value in the accent colour, for the one line that matters most. */
  strong?: boolean;
}

/**
 * The dialog an action reports through: 整合入库 and 清空 both end here instead of
 * leaving a one-line notice in the status bar, because the requirement is to say
 * what happened and with which numbers.
 */
export function SummaryDialog({
  open,
  title,
  items,
  tone = 'success',
  onClose,
}: {
  open: boolean;
  title: string;
  items: SummaryItem[];
  tone?: 'success' | 'neutral';
  onClose: () => void;
}) {
  const accent = tone === 'success' ? JNJ.success : JNJ.red;
  return (
    <Modal open={open} onCancel={onClose} onOk={onClose} okText="知道了" cancelButtonProps={{ style: { display: 'none' } }} width={460} title={title}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: '4px 0 2px' }}>
        {items.map((i) => (
          <div key={i.label} style={{ display: 'flex', gap: 12, alignItems: 'baseline' }}>
            <span style={{ width: 84, flex: '0 0 auto', color: JNJ.textMuted, fontSize: 12 }}>{i.label}</span>
            <span
              style={{
                fontWeight: i.strong ? 600 : 400,
                color: i.strong ? accent : JNJ.ink,
                wordBreak: 'break-all',
              }}
            >
              {i.value}
            </span>
          </div>
        ))}
      </div>
    </Modal>
  );
}
