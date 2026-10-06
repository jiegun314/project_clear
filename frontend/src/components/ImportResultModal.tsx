import { Modal, Table, Tag, Statistic, Alert, Space, Typography } from 'antd';
import { CheckCircle2, XCircle } from 'lucide-react';
import { JNJ } from '../theme/jnj';
import type { FileResult, ImportResult } from '../types';

export interface ImportResultModalProps {
  open: boolean;
  result: ImportResult | null;
  onClose: () => void;
}

/** After every import or add, the brief asks for a success/failure breakdown. */
export function ImportResultModal({ open, result, onClose }: ImportResultModalProps) {
  if (!result) return null;
  const failed = result.failed;
  const tone = failed === 0 ? JNJ.success : result.ok === 0 ? JNJ.danger : JNJ.warning;

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
          {result.action === 'import' ? '导入完成' : '添加完成'}
          {failed > 0 ? `（${failed} 个文件失败）` : ''}
        </span>
      }
    >
      <Space size={40} style={{ margin: '8px 0 16px' }}>
        <Statistic title="处理文件" value={result.total} />
        <Statistic title="成功" value={result.ok} styles={{ content: { color: JNJ.success } }} />
        <Statistic title="失败" value={result.failed} styles={{ content: { color: failed ? JNJ.danger : undefined } }} />
        <Statistic title="合并行数" value={result.rowsKept} />
        <Statistic title="耗时" value={result.durationMs} suffix="ms" />
      </Space>

      <Space style={{ marginBottom: 12 }} wrap>
        <Tag color={JNJ.red.replace('#', '')} style={{ color: '#fff', background: JNJ.red, border: 'none' }}>
          周码 {result.weekCode}
        </Tag>
        <Tag>起始日期 {result.weekStart}</Tag>
        <Tag>周列 {result.weekCodes?.length ?? 0} 个</Tag>
        {result.weekCodes?.length ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {result.weekCodes.map((w) => w.code).join(' · ')}
          </Typography.Text>
        ) : null}
      </Space>

      {result.warnings?.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          title={`${result.warnings.length} 条提示`}
          description={
            <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
              {result.warnings.slice(0, 8).map((w) => (
                <li key={w} style={{ fontSize: 12 }}>
                  {w}
                </li>
              ))}
            </ul>
          }
        />
      )}

      <Table<FileResult>
        size="small"
        bordered
        rowKey="name"
        dataSource={result.files ?? []}
        pagination={false}
        scroll={{ y: 280 }}
        columns={[
          {
            title: '文件',
            dataIndex: 'name',
            ellipsis: true,
            render: (v: string, r) => (
              <span title={r.path}>
                {v}
                {r.err ? <div style={{ color: JNJ.danger, fontSize: 11 }}>{r.err}</div> : null}
              </span>
            ),
          },
          {
            title: '状态',
            dataIndex: 'status',
            width: 80,
            // Centred both ways. antd's `align` is horizontal only, and a table
            // cell defaults to `vertical-align: baseline` — so a row made tall by
            // the file column (a name, and an error line under it) used to leave
            // the chip stranded at the top of its cell.
            align: 'center',
            onCell: () => ({ style: { verticalAlign: 'middle' } }),
            filters: [
              { text: '成功', value: 'ok' },
              { text: '失败', value: 'failed' },
            ],
            onFilter: (v, r) => r.status === v,
            render: (v: string) =>
              v === 'ok' ? (
                // The preset `color="success"` fills the tag with a pale tint
                // derived from our own dark green, which reads as grey rather than
                // as "this worked". The lightest grey fill with the success green
                // as the text is calmer and plainly readable — and it keeps the
                // same fill the theme already gives a plain Tag.
                <Tag style={{ background: JNJ.fill, color: JNJ.success, border: 'none', margin: 0 }}>
                  成功
                </Tag>
              ) : (
                // `margin: 0` as well: antd's tag margin would push the chip off
                // centre inside a centred cell.
                <Tag color="error" style={{ margin: 0 }}>
                  失败
                </Tag>
              ),
          },
          { title: '数据行', dataIndex: 'rowsTotal', width: 90, align: 'right' },
          {
            title: `命中 LOC`,
            dataIndex: 'rowsKept',
            width: 90,
            align: 'right',
            render: (v: number) => <span style={{ fontWeight: 600, color: v > 0 ? JNJ.ink : JNJ.textMuted }}>{v}</span>,
          },
        ]}
        summary={(rows) => {
          const kept = rows.reduce((a, r) => a + r.rowsKept, 0);
          return (
            <Table.Summary fixed>
              <Table.Summary.Row style={{ background: JNJ.surfaceAlt }}>
                <Table.Summary.Cell index={0}>
                  <b>合计</b>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={1} align="right">
                  {rows.filter((r) => r.status === 'ok').length} / {rows.length}
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  {rows.reduce((a, r) => a + r.rowsTotal, 0)}
                </Table.Summary.Cell>
                <Table.Summary.Cell index={3} align="right">
                  <b>{kept}</b>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            </Table.Summary>
          );
        }}
      />
    </Modal>
  );
}
