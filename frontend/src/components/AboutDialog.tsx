import { useEffect, useState } from 'react';
import { Modal, Descriptions, Typography, Space } from 'antd';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { AppInfo } from '../types';

export interface AboutDialogProps {
  open: boolean;
  onClose: () => void;
}

export function AboutDialog({ open, onClose }: AboutDialogProps) {
  const [info, setInfo] = useState<AppInfo | null>(null);

  useEffect(() => {
    if (!open) return;
    api.getAppInfo().then(setInfo).catch(() => setInfo(null));
  }, [open]);

  return (
    <Modal open={open} onCancel={onClose} onOk={onClose} okText="关闭" cancelButtonProps={{ style: { display: 'none' } }} width={520} footer={null}>
      <div style={{ textAlign: 'center', padding: '8px 0 4px' }}>
        <img
          src="/clear.png"
          alt="CLEAR"
          onError={(e) => {
            (e.currentTarget as HTMLImageElement).style.display = 'none';
          }}
          style={{ width: 84, height: 84, objectFit: 'contain' }}
        />
        <div style={{ fontSize: 26, fontWeight: 700, color: JNJ.red, letterSpacing: 4, marginTop: 6 }}>
          CLEAR
        </div>
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 2 }}>
          {info?.fullName ?? 'Consolidation & Loading of Enterprise Analytics for Replenishment'}
        </Typography.Text>
        <div style={{ marginTop: 6, color: JNJ.text, fontSize: 13 }}>MPS 数据整合与补货分析平台</div>
        <div style={{ marginTop: 2, color: JNJ.textMuted, fontSize: 12 }}>版本 {info?.version ?? '—'}</div>
      </div>

      <Descriptions
        column={1}
        size="small"
        bordered
        style={{ marginTop: 16 }}
        labelStyle={{ width: 92, color: JNJ.text, fontSize: 12 }}
        contentStyle={{ fontSize: 12 }}
        items={[
          { key: 'd', label: '数据目录', children: info?.dataDir ?? '—' },
          { key: 'c', label: '参数文件', children: info?.configPath ?? '—' },
          { key: 'db', label: '数据库', children: info?.database ?? '—' },
          { key: 'g', label: '运行环境', children: `${info?.goVersion ?? ''} · ${info?.platform ?? ''}` },
        ]}
      />
      <Space direction="vertical" size={2} style={{ marginTop: 12, width: '100%' }}>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>
          技术方案：Go + Wails + Excelize，前端 React + Ant Design。
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>
          每周整合一次，永久数据按周码命名（如 2639），重复整合将覆盖原表。
        </Typography.Text>
      </Space>
    </Modal>
  );
}
