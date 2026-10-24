import { useEffect, useState } from 'react';
import { Alert, Modal, Radio, Space } from 'antd';
import { JNJ } from '../theme/jnj';

export interface MergeRangeDialogProps {
  open: boolean;
  /** Week columns available in the current Global/Local datasets. */
  globalWeeks: number;
  localWeeks: number;
  /** Preselected cutoff: 26 (AO) or 61 (BX). */
  defaultWeeks?: number;
  onCancel: () => void;
  onConfirm: (weeks: number) => void;
}

/**
 * The integration range choice. The source data is imported up to BX, so the
 * cutoff can change per integration without re-reading the workbooks.
 */
export function MergeRangeDialog({
  open,
  globalWeeks,
  localWeeks,
  defaultWeeks = 26,
  onCancel,
  onConfirm,
}: MergeRangeDialogProps) {
  const [weeks, setWeeks] = useState(defaultWeeks);
  useEffect(() => {
    if (open) setWeeks(defaultWeeks);
  }, [open, defaultWeeks]);

  const available = Math.min(globalWeeks, localWeeks);
  const short = weeks > available;
  return (
    <Modal
      open={open}
      onCancel={onCancel}
      onOk={() => onConfirm(weeks)}
      okText="开始整合"
      cancelText="取消"
      width={460}
      title="选择本次整合的截止范围"
    >
      <Space orientation="vertical" size={12} style={{ width: '100%' }}>
        <div style={{ color: JNJ.text, fontSize: 13 }}>
          数据已按 P 列起最多 61 周导入。AO 为前 26 个周列，BX 为前 61 个周列；
          本次选择会同时作用于 Global 和 Local。
        </div>
        <Radio.Group
          value={weeks}
          onChange={(e) => setWeeks(e.target.value as number)}
          style={{ display: 'flex', flexDirection: 'column', gap: 6 }}
        >
          <Radio value={26}>AO：截止到第 26 个周列（常规）</Radio>
          <Radio value={61}>BX：截止到第 61 个周列（扩展）</Radio>
        </Radio.Group>
        <div style={{ fontSize: 12, color: JNJ.textMuted }}>
          当前 Global 有 {globalWeeks} 周，Local 有 {localWeeks} 周。
        </div>
        {short && (
          <Alert
            type="warning"
            showIcon
            title={`选择 ${weeks} 周，但当前交集最多 ${available} 周；整合将按实际交集进行，并在结果中提示。`}
          />
        )}
      </Space>
    </Modal>
  );
}
