import { useEffect, useState } from 'react';
import {
  Modal,
  Form,
  InputNumber,
  Input,
  Select,
  Radio,
  Alert,
  Typography,
  Space,
  Divider,
} from 'antd';
import { FolderCog, RotateCcw, Save } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { AppConfig } from '../types';

export interface SettingsDialogProps {
  open: boolean;
  onClose: () => void;
  onSaved: (c: AppConfig) => void;
}

/** The parameter screen. Everything here is persisted to the YAML file. */
export function SettingsDialog({ open, onClose, onSaved }: SettingsDialogProps) {
  const [cfg, setCfg] = useState<AppConfig | null>(null);
  const [path, setPath] = useState('');
  const [notes, setNotes] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    api
      .getConfig()
      .then((v) => {
        setCfg(v.config);
        setPath(v.path);
        setNotes([]);
      })
      .catch(() => setCfg(null));
  }, [open]);

  if (!cfg) return null;

  const save = async () => {
    setSaving(true);
    try {
      const n = await api.saveConfig(cfg);
      setNotes(n ?? []);
      const v = await api.getConfig();
      setCfg(v.config);
      onSaved(v.config);
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    setSaving(true);
    try {
      const v = await api.resetConfig();
      setCfg(v.config);
      setNotes(['已恢复出厂默认参数']);
      onSaved(v.config);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      width={560}
      title={
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <FolderCog size={18} style={{ color: JNJ.red }} />
          参数设定
        </span>
      }
      footer={[
        <Space key="f" style={{ justifyContent: 'space-between', width: '100%' }}>
          <a key="r" onClick={reset} style={{ color: JNJ.text }}>
            <RotateCcw size={13} style={{ verticalAlign: -2, marginRight: 4 }} />
            恢复默认
          </a>
          <Space key="b">
            <a key="c" onClick={onClose} style={{ color: JNJ.text }}>
              取消
            </a>
            <button
              key="s"
              className="ant-btn ant-btn-primary"
              disabled={saving}
              onClick={save}
              style={{ background: JNJ.red, borderColor: JNJ.red }}
            >
              <Save size={14} style={{ verticalAlign: -2, marginRight: 4 }} />
              保存
            </button>
          </Space>
        </Space>,
      ]}
    >
      {notes.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 14 }}
          message="参数已自动修正"
          description={
            <ul style={{ margin: '4px 0 0', paddingLeft: 18 }}>
              {notes.map((n) => (
                <li key={n} style={{ fontSize: 12 }}>
                  {n}
                </li>
              ))}
            </ul>
          }
        />
      )}

      <Form layout="vertical" size="middle">
        <Form.Item
          label="读取列数（从 P 列开始）"
          extra="源文件第 55/56 行的周码与起始日期列数，默认 20，即 P..AI"
        >
          <InputNumber
            min={1}
            max={200}
            value={cfg.readColumns}
            onChange={(v) => setCfg({ ...cfg, readColumns: v ?? 20 })}
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Form.Item label="LOC 筛选值（L 列）" extra="只保留该 LOC 的数据行，留空将回退为 WH_CNB">
          <Input
            value={cfg.locFilter}
            onChange={(e) => setCfg({ ...cfg, locFilter: e.target.value })}
            placeholder="WH_CNB"
          />
        </Form.Item>

        <Form.Item label="每页显示行数">
          <InputNumber
            min={10}
            max={5000}
            step={50}
            value={cfg.pageSize}
            onChange={(v) => setCfg({ ...cfg, pageSize: v ?? 200 })}
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Divider style={{ margin: '4px 0 16px' }} />

        <Form.Item
          label="导出方式"
          extra="模板改写：以首个源文件为模板改写，保留全部样式、条件格式、注释与宏；干净重建：只输出 MPS 表，不含宏"
        >
          <Radio.Group
            value={cfg.exportMode}
            onChange={(e) => setCfg({ ...cfg, exportMode: e.target.value })}
          >
            <Radio value="template" style={{ marginBottom: 6 }}>
              模板改写（.xlsm，保真最高）
            </Radio>
            <Radio value="clean">干净重建（.xlsx，体积小）</Radio>
          </Radio.Group>
        </Form.Item>

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          配置文件：{path}
        </Typography.Text>
      </Form>
    </Modal>
  );
}
