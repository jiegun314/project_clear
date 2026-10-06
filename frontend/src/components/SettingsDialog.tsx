import { useEffect, useState } from 'react';
import { Modal, Form, InputNumber, Input, Radio, Alert, Typography, Button, Tooltip, Spin, Empty } from 'antd';
import { FolderCog, RotateCcw, Check, X } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import { errorText } from '../lib/errors';
import type { AppConfig } from '../types';

export interface SettingsDialogProps {
  open: boolean;
  onClose: () => void;
  onSaved: (c: AppConfig) => void;
}

/**
 * One parameter block. Blocks are separated by a hairline in a single flat
 * colour, drawn flush with the modal padding: antd's Divider leaves a gap for
 * its label and paints a gradient, which looked busy between short rows.
 */
function Section({ first, children }: { first?: boolean; children: React.ReactNode }) {
  return (
    <div
      style={{
        padding: first ? '0 0 12px' : '12px 0',
        borderTop: first ? undefined : `1px solid ${JNJ.divider}`,
      }}
    >
      {children}
    </div>
  );
}

/** The parameter screen. Everything here is persisted to the YAML file. */
export function SettingsDialog({ open, onClose, onSaved }: SettingsDialogProps) {
  const [cfg, setCfg] = useState<AppConfig | null>(null);
  const [path, setPath] = useState('');
  const [notes, setNotes] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  // Loading and failure are states of this dialog, not reasons to disappear: the
  // old `if (!cfg) return null` meant a failed load left the window invisible
  // with no explanation, and the button appeared to do nothing.
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError('');
    api
      .getConfig()
      .then((v) => {
        setCfg(v.config);
        setPath(v.path);
        setNotes([]);
      })
      .catch((e) => {
        setCfg(null);
        setError(errorText(e));
      })
      .finally(() => setLoading(false));
  }, [open]);

  if (!open) return null;

  const save = async () => {
    if (!cfg) return;
    setSaving(true);
    setError('');
    try {
      const n = await api.saveConfig(cfg);
      setNotes(n ?? []);
      const v = await api.getConfig();
      setCfg(v.config);
      onSaved(v.config);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    setSaving(true);
    setError('');
    try {
      const v = await api.resetConfig();
      setCfg(v.config);
      setNotes(['已恢复出厂默认参数']);
      onSaved(v.config);
    } catch (e) {
      setError(errorText(e));
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
      footer={
        // Icon-only buttons, same shape as the toolbar's: the tooltip and the
        // aria-label carry the wording the buttons no longer show.
        <div style={{ display: 'flex', alignItems: 'center', width: '100%' }}>
          <Tooltip title="恢复默认参数" mouseEnterDelay={0.15}>
            <Button
              icon={<RotateCcw size={16} />}
              onClick={reset}
              disabled={saving}
              aria-label="恢复默认参数"
              style={{ color: JNJ.text }}
            />
          </Tooltip>
          <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
            <Tooltip title="取消" mouseEnterDelay={0.15}>
              <Button
                icon={<X size={16} />}
                onClick={onClose}
                aria-label="取消"
                style={{ color: JNJ.text }}
              />
            </Tooltip>
            <Tooltip title="保存参数" mouseEnterDelay={0.15}>
              <Button
                type="primary"
                icon={<Check size={16} />}
                onClick={save}
                disabled={saving || !cfg}
                loading={saving}
                aria-label="保存参数"
                style={{ background: JNJ.red, borderColor: JNJ.red }}
              />
            </Tooltip>
          </div>
        </div>
      }
    >
      {loading && (
        <div style={{ padding: '32px 0', textAlign: 'center' }}>
          <Spin />
        </div>
      )}

      {!loading && !cfg && (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={
            <span style={{ color: JNJ.danger }}>
              {error ? `参数读取失败：${error}` : '参数读取失败'}
            </span>
          }
        />
      )}

      {!loading && cfg && error && (
        <Alert type="error" showIcon style={{ marginBottom: 14 }} title="操作失败" description={error} />
      )}

      {!loading && cfg && notes.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 14 }}
          title="参数已自动修正"
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

      {!loading && cfg && (
      <Form layout="vertical" size="middle">
        <Section first>
          <Form.Item
            label="读取列数（从 P 列开始）"
            extra="源文件第 55/56 行的周码与起始日期列数，默认 20，即 P..AI"
            style={{ marginBottom: 0 }}
          >
            <InputNumber
              min={1}
              max={200}
              value={cfg.readColumns}
              onChange={(v) => setCfg({ ...cfg, readColumns: v ?? 20 })}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </Section>

        <Section>
          <Form.Item
            label="LOC 筛选值（L 列）"
            extra="只保留该 LOC 的数据行，留空将回退为 WH_CNB"
            style={{ marginBottom: 0 }}
          >
            <Input
              value={cfg.locFilter}
              onChange={(e) => setCfg({ ...cfg, locFilter: e.target.value })}
              placeholder="WH_CNB"
            />
          </Form.Item>
        </Section>

        <Section>
          <Form.Item label="每页显示行数" style={{ marginBottom: 0 }}>
            <InputNumber
              min={10}
              max={5000}
              step={50}
              value={cfg.pageSize}
              onChange={(v) => setCfg({ ...cfg, pageSize: v ?? 200 })}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </Section>

        <Section>
          <Form.Item
            label="表头显示"
            extra="主表格的周列表头：两行显示周码与起始日期，一行只显示周码"
            style={{ marginBottom: 0 }}
          >
            <Radio.Group
              value={cfg.headerDisplay}
              onChange={(e) => setCfg({ ...cfg, headerDisplay: e.target.value })}
            >
              <Radio value="twoRow" style={{ marginBottom: 6 }}>
                两行（默认：周码 + 起始日期）
              </Radio>
              <Radio value="oneRow">一行（只有周码，表头更紧凑）</Radio>
            </Radio.Group>
          </Form.Item>
        </Section>

        <Section>
          <Form.Item
            label="导出方式"
            extra="两种方式都只输出 MPS 页，都保留颜色、条件格式与备注"
            style={{ marginBottom: 0 }}
          >
            <Radio.Group
              value={cfg.exportMode}
              onChange={(e) => setCfg({ ...cfg, exportMode: e.target.value })}
            >
              <Radio value="clean" style={{ marginBottom: 6 }}>
                纯数据（默认，.xlsx：只有表头与全部数据，不含宏）
              </Radio>
              <Radio value="template">原文件格式（.xlsm：沿用源文件样式与宏）</Radio>
            </Radio.Group>
          </Form.Item>
        </Section>

        <Section>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            配置文件：{path}
          </Typography.Text>
        </Section>
      </Form>
      )}
    </Modal>
  );
}
