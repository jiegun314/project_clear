import { Modal, Table, Tag, Tooltip, Empty } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { JNJ } from '../theme/jnj';
import type { StagedFile, StagingFilesView } from '../types';

export interface StagedFilesModalProps {
  open: boolean;
  view: StagingFilesView | null;
  onClose: () => void;
}

/**
 * The list behind 已导入文件: every workbook that has been imported or added
 * and is waiting to be integrated, with the number of rows it contributed.
 * 清空 empties it, and re-adding a file replaces its line instead of adding a
 * second one.
 */
export function StagedFilesModal({ open, view, onClose }: StagedFilesModalProps) {
  const files = view?.files ?? [];
  const columns: ColumnsType<StagedFile> = [
    {
      title: '序号',
      key: 'index',
      width: 56,
      align: 'center',
      render: (_v, _row, index) => (
        <span style={{ color: JNJ.textMuted, fontVariantNumeric: 'tabular-nums' }}>
          {index + 1}
        </span>
      ),
    },
    {
      // Only the header is centred here: the name itself reads best left
      // aligned and gets all the room the two narrow columns do not need.
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
        row.status === 'ok' ? (
          <span style={{ fontVariantNumeric: 'tabular-nums' }}>{n.toLocaleString()}</span>
        ) : (
          '—'
        ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 76,
      align: 'center',
      render: (s: string, row) =>
        s === 'ok' ? (
          <Tag color="green" style={{ margin: 0 }}>
            已导入
          </Tag>
        ) : (
          <Tooltip title={row.err}>
            <Tag color="red" style={{ margin: 0 }}>
              读取失败
            </Tag>
          </Tooltip>
        ),
    },
  ];

  return (
    <Modal
      open={open}
      onCancel={onClose}
      onOk={onClose}
      okText="关闭"
      cancelButtonProps={{ style: { display: 'none' } }}
      width={700}
      title={
        <span>
          已导入文件
          <span style={{ marginLeft: 10, fontWeight: 400, fontSize: 12, color: JNJ.textMuted }}>
            {view?.fileCount ?? 0} 个文件 · {(view?.rowCount ?? 0).toLocaleString()} 行
            {view && view.failedCount > 0 ? ` · ${view.failedCount} 个读取失败` : ''}
          </span>
        </span>
      }
    >
      {/* One table with a sticky header inside our own scroll box. antd's
          scroll.y splits the header and the body into two tables, and the
          body's scrollbar then eats into the last column so the 状态 header no
          longer lines up with its cells. A single table cannot drift. */}
      <div style={{ maxHeight: 360, overflowY: 'auto' }}>
        <Table<StagedFile>
          size="small"
          bordered
          sticky
          tableLayout="fixed"
          rowKey={(r) => r.path || r.name}
          columns={columns}
          dataSource={files}
          pagination={false}
          locale={{
            emptyText: (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有导入任何文件" />
            ),
          }}
        />
      </div>
      <div style={{ marginTop: 8, fontSize: 12, color: JNJ.textMuted }}>
        同名文件只会有一行；再次导入或添加同名文件会直接覆盖它原来的数据（其余文件不受影响），点清空后本列表清空。
      </div>
    </Modal>
  );
}
