import { Modal, Table, Tag, Statistic, Alert, Space, Typography } from 'antd';
import { CheckCircle2, XCircle } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import type { FileResult, ImportResult } from '../types';

export interface ImportResultModalProps {
  open: boolean;
  result: ImportResult | null;
  onClose: () => void;
}

/** The title, per action: a Global draft, a Local dataset, or a discard. */
function actionTitle(action: string): string {
  switch (action) {
    case 'import':
    case 'add':
      return '已加入 Global 导入草稿';
    case 'local':
      return 'Local 导入完成';
    case 'discard':
      return '已丢弃导入草稿';
    default:
      return '操作完成';
  }
}

/** After every import/add, the brief asks for a success/failure breakdown. */
export function ImportResultModal({ open, result, onClose }: ImportResultModalProps) {
  if (!result) return null;
  const failed = result.failed;
  const tone = failed === 0 ? JNJ.success : result.ok === 0 ? JNJ.danger : JNJ.warning;
  const draft = result.draft;

  return (
    <Modal
      open={open}
      onCancel={onClose}
      onOk={onClose}
      okText="知道了"
      cancelButtonProps={{ style: { display: 'none' } }}
      width={880}
      title={
        <span style={{ color: tone, display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          {failed === 0 ? <CheckCircle2 size={18} /> : <XCircle size={18} />}
          {actionTitle(result.action)}
          {failed > 0 ? `（${failed} 个文件失败）` : ''}
        </span>
      }
    >
      <Space size={40} style={{ margin: '8px 0 16px' }}>
        <Statistic title="处理文件" value={result.total} />
        <Statistic title="成功" value={result.ok} styles={{ content: { color: JNJ.success } }} />
        <Statistic title="失败" value={result.failed} styles={{ content: { color: failed ? JNJ.danger : undefined } }} />
        <Statistic title="导入行数" value={result.rowsKept} />
        <Statistic title="耗时" value={result.durationMs} suffix="ms" />
      </Space>

      <Space style={{ marginBottom: 12 }} wrap>
        <Tag color={JNJ.red.replace('#', '')} style={{ color: '#fff', background: JNJ.red, border: 'none' }}>
          周码 {result.weekCode}
        </Tag>
        <Tag>起始日期 {result.weekStart}</Tag>
        <Tag>周列 {result.weekCodes?.length ?? 0} 个</Tag>
        {draft ? (
          <Tag style={{ background: JNJ.warningSoft, color: JNJ.warning, borderColor: '#F0DCB8' }}>草稿（待完成导入）</Tag>
        ) : (
          <Tag style={{ background: JNJ.successSoft, color: JNJ.success, borderColor: '#CBE6D5' }}>Local v{result.version}</Tag>
        )}
        {result.expired && (
          <Tag style={{ background: JNJ.warningSoft, color: JNJ.warning, borderColor: '#F0DCB8' }}>整合已过期</Tag>
        )}
        {result.weekCodes?.length ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {result.weekCodes.map((w) => w.code).join(' · ')}
          </Typography.Text>
        ) : null}
      </Space>

      {draft && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          title="这是 Global 导入草稿"
          description="点工具栏的「完成 Global 导入」后，这份数据才会成为可整合的数据集；完成前「整合」不可用。"
        />
      )}
      {result.expired && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          title="该周已有整合记录，现在已过期"
          description="新的导入已保存；历史里的整合仍指向旧版本，重新整合后才会更新。"
        />
      )}
      {result.warnings?.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          title={`${result.warnings.length} 条提示`}
          description={
            <ul style={{ margin: 0, paddingLeft: 18 }}>
              {result.warnings.map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          }
        />
      )}

      {result.action !== 'discard' && (
        <Table<FileResult>
          size="small"
          bordered
          rowKey={(r) => r.path || r.name}
          pagination={false}
          dataSource={result.files}
          columns={[
            { title: '文件', dataIndex: 'name', key: 'name', ellipsis: true },
            {
              title: '读取行数',
              dataIndex: 'rowsTotal',
              key: 'rowsTotal',
              width: 100,
              align: 'center',
              render: (n: number) => n.toLocaleString(),
            },
            {
              title: '命中行数',
              dataIndex: 'rowsKept',
              key: 'rowsKept',
              width: 100,
              align: 'center',
              render: (n: number) => n.toLocaleString(),
            },
            {
              title: '状态',
              dataIndex: 'status',
              key: 'status',
              width: 90,
              align: 'center',
              render: (s: string) =>
                s === 'ok' ? <Tag color="green">成功</Tag> : <Tag color="red">失败</Tag>,
            },
          ]}
        />
      )}
    </Modal>
  );
}
