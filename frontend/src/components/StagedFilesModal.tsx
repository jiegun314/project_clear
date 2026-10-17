import { Modal, Table, Tag, Tooltip, Empty } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { JNJ } from '../theme/jnj';
import type { FileWithDataset, FilesView } from '../types';

export interface StagedFilesModalProps {
  open: boolean;
  view: FilesView | null;
  onClose: () => void;
}

function DatasetLabel({ dataset }: { dataset: string }) {
  return (
    <Tag
      style={{
        margin: 0,
        padding: '0 6px',
        fontSize: 11,
        lineHeight: '18px',
        background: JNJ.surface,
        borderColor: JNJ.border,
        color: JNJ.text,
      }}
    >
      {dataset}
    </Tag>
  );
}

/**
 * The list behind 已导入文件: the workbooks of the current Global draft (or the
 * completed Global) and of the Local dataset, with the rows each contributed.
 */
export function StagedFilesModal({ open, view, onClose }: StagedFilesModalProps) {
  const files = view?.files ?? [];
  const okFiles = files.filter((f) => f.status === 'ok');
  // Prefer the dataset totals: a migrated week has no per-file rows, and a draft
  // with a failed file must still show the counts the status strip uses.
  const rows = view?.rowCount ?? okFiles.reduce((n, f) => n + f.rowsKept, 0);
  const fileCount = view?.fileCount ?? okFiles.length;
  const failed = view?.failedCount ?? files.length - okFiles.length;

  const columns: ColumnsType<FileWithDataset> = [
    {
      title: '数据集',
      dataIndex: 'dataset',
      key: 'dataset',
      width: 130,
      render: (d: string) => <DatasetLabel dataset={d} />,
    },
    {
      title: <span style={{ display: 'block', textAlign: 'center' }}>文件名</span>,
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
      render: (name: string, row) => (
        <Tooltip title={row.path}>
          <span style={{ color: row.status === 'ok' ? JNJ.ink : JNJ.textMuted }}>{name}</span>
        </Tooltip>
      ),
    },
    {
      title: '导入行数',
      dataIndex: 'rowsKept',
      key: 'rowsKept',
      width: 88,
      align: 'center',
      render: (n: number, row) =>
        row.status === 'ok' ? <span style={{ fontVariantNumeric: 'tabular-nums' }}>{n.toLocaleString()}</span> : '—',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 86,
      align: 'center',
      render: (s: string, row) => {
        if (s !== 'ok') {
          return (
            <Tooltip title={row.err}>
              <Tag color="red" style={{ margin: 0 }}>
                读取失败
              </Tag>
            </Tooltip>
          );
        }
        // While the Global import is still a draft every readable workbook is
        // 导入中; only 完成 Global 导入 makes them 已导入.
        return view?.hasDraft ? (
          <Tag color="gold" style={{ margin: 0 }}>
            导入中
          </Tag>
        ) : (
          <Tag color="green" style={{ margin: 0 }}>
            已导入
          </Tag>
        );
      },
    },
  ];

  return (
    <Modal
      open={open}
      onCancel={onClose}
      onOk={onClose}
      okText="关闭"
      cancelButtonProps={{ style: { display: 'none' } }}
      width={760}
      title={
        <span>
          Global 文件清单
          <span style={{ marginLeft: 10, fontWeight: 400, fontSize: 12, color: JNJ.textMuted }}>
            {fileCount} 个文件 · {rows.toLocaleString()} 行
            {failed > 0 ? ` · ${failed} 个读取失败` : ''}
            {view?.hasDraft ? ' · Global 导入尚未完成' : ''}
          </span>
        </span>
      }
    >
      {view?.note && (
        <div style={{ marginBottom: 8, fontSize: 12, color: JNJ.textMuted }}>{view.note}</div>
      )}
      <div style={{ maxHeight: 360, overflowY: 'auto' }}>
        <Table<FileWithDataset>
          size="small"
          bordered
          sticky
          tableLayout="fixed"
          rowKey={(r) => `${r.dataset}:${r.path || r.name}`}
          columns={columns}
          dataSource={files}
          pagination={false}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前没有导入文件" /> }}
        />
      </div>
      <div style={{ marginTop: 8, fontSize: 12, color: JNJ.textMuted }}>
        Global 导入是草稿：点「完成 Global 导入」后才会成为可整合的数据集；Local 一次导入即完成。
        同名文件只会有一行，再次导入会覆盖它原来的数据。Local 文件一次导入即完成，
        它的文件信息显示在状态条中，不在这里。
      </div>
    </Modal>
  );
}
