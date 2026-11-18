import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  AutoComplete,
  Button,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { TableProps } from 'antd';
import {
  ArrowLeft,
  BookOpen,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  Upload,
} from 'lucide-react';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type {
  MasterImportResult,
  MasterInput,
  MasterOverview,
  MasterRow,
  MasterTable,
} from '../types';

type TabKey = MasterTable | 'price';

interface Props {
  onClose: () => void;
}

interface EditorValues {
  itemCode?: string;
  baseCode?: string;
  manuCode?: string;
  sapCode?: string;
  category?: string;
}

interface EditorState {
  mode: 'new' | 'edit';
  row?: MasterRow;
}

const TABLE_LABEL: Record<MasterTable, string> = {
  item: 'Item → Base',
  base: 'Base → SAP',
  category: 'SAP → Category',
};

const PAGE_SIZE = 50;

function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err || '操作失败');
}

function isFormError(err: unknown): boolean {
  return Boolean(err && typeof err === 'object' && 'errorFields' in err);
}

function CategoryTag({ category }: { category?: string }) {
  if (!category) return <Tag color="orange">未分类</Tag>;
  return <Tag>{category}</Tag>;
}

/**
 * 主数据: the code matching and category tables, kept on this machine.
 *
 * The page is deliberately separate from the weekly workbench. These mappings
 * are overwritten in place and are not retroactively applied to old weeks; a new
 * import picks them up from that point on.
 */
export function MasterDataView({ onClose }: Props) {
  const [tab, setTab] = useState<TabKey>('item');
  const [overview, setOverview] = useState<MasterOverview | null>(null);
  const [rows, setRows] = useState<MasterRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState('all');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmImport, setConfirmImport] = useState(false);
  const [importing, setImporting] = useState(false);
  const [importResult, setImportResult] = useState<MasterImportResult | null>(null);
  const [form] = Form.useForm<EditorValues>();
  const activeLabel = tab === 'price' ? '产品价格' : TABLE_LABEL[tab];

  const loadOverview = useCallback(async () => {
    try {
      setOverview(await api.getMasterOverview());
    } catch (err) {
      setError(errorText(err));
    }
  }, []);

  const loadRows = useCallback(async () => {
    if (tab === 'price') return;
    setLoading(true);
    try {
      const result = await api.queryMasterData({
        table: tab,
        page,
        pageSize: PAGE_SIZE,
        search,
        filter,
      });
      setRows(result.rows);
      setTotal(result.total);
      setError('');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setLoading(false);
    }
  }, [tab, page, search, filter]);

  const refresh = useCallback(async () => {
    await Promise.all([loadOverview(), loadRows()]);
  }, [loadOverview, loadRows]);

  useEffect(() => {
    void loadOverview();
  }, [loadOverview]);

  useEffect(() => {
    void loadRows();
  }, [loadRows]);

  const changeTab = (next: TabKey) => {
    setTab(next);
    setPage(1);
    setSearch('');
    setFilter('all');
    setNotice('');
    setEditor(null);
  };

  const openNew = () => {
    form.resetFields();
    setEditor({ mode: 'new' });
  };

  const openEdit = useCallback((row: MasterRow) => {
    form.setFieldsValue({
      itemCode: row.itemCode,
      baseCode: row.baseCode,
      manuCode: row.manuCode,
      sapCode: row.sapCode,
      category: row.category,
    });
    setEditor({ mode: 'edit', row });
  }, [form]);

  const save = async () => {
    if (!editor || tab === 'price') return;
    let values: EditorValues;
    try {
      values = await form.validateFields();
    } catch (err) {
      if (!isFormError(err)) setError(errorText(err));
      return;
    }
    const input: MasterInput = {
      table: tab,
      originalKey: editor.mode === 'edit' ? editor.row?.key ?? '' : '',
      itemCode: values.itemCode?.trim() ?? '',
      baseCode: values.baseCode?.trim() ?? '',
      manuCode: values.manuCode?.trim() ?? '',
      sapCode: values.sapCode?.trim() ?? '',
      category: values.category?.trim() ?? '',
    };
    setSaving(true);
    try {
      await api.saveMasterRow(input);
      setEditor(null);
      setNotice(`已保存 ${activeLabel} 映射`);
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = useCallback((row: MasterRow) => {
    Modal.confirm({
      title: `删除这条 ${activeLabel} 记录？`,
      content: row.key,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.deleteMasterRow(tab as MasterTable, row.key);
          setNotice('记录已删除');
          await refresh();
        } catch (err) {
          setError(errorText(err));
        }
      },
    });
  }, [activeLabel, refresh, tab]);

  const runImport = async () => {
    setConfirmImport(false);
    setImporting(true);
    try {
      const result = await api.importMasterWorkbook();
      if (!result) return;
      setImportResult(result);
      setPage(1);
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setImporting(false);
    }
  };

  const filterOptions = useMemo(() => {
    switch (tab) {
      case 'item':
        return [{ value: 'all', label: '全部' }, { value: 'orphan', label: '仅孤儿 Base' }];
      case 'base':
        return [{ value: 'all', label: '全部' }, { value: 'uncategorized', label: '仅未分类' }];
      case 'category':
        return [{ value: 'all', label: '全部' }, { value: 'unused', label: '仅未使用' }];
      default:
        return [{ value: 'all', label: '全部' }];
    }
  }, [tab]);

  const actions = useCallback((row: MasterRow) => (
    <Space size={2}>
      <Tooltip title="编辑">
        <Button
          type="text"
          size="small"
          icon={<Pencil size={15} />}
          aria-label={`编辑 ${row.key}`}
          onClick={() => openEdit(row)}
        />
      </Tooltip>
      <Tooltip title="删除">
        <Button
          type="text"
          size="small"
          danger
          icon={<Trash2 size={15} />}
          aria-label={`删除 ${row.key}`}
          onClick={() => remove(row)}
        />
      </Tooltip>
    </Space>
  ), [openEdit, remove]);

  const columns = useMemo<TableProps<MasterRow>['columns']>(() => {
    const updated = {
      title: '更新时间',
      dataIndex: 'updatedAt',
      width: 170,
      render: (v: string) => <span style={{ color: JNJ.textMuted }}>{v || '—'}</span>,
    };
    const ops = { title: '操作', key: 'ops', width: 86, render: (_: unknown, row: MasterRow) => actions(row) };
    if (tab === 'item') {
      return [
        { title: 'Item Code', dataIndex: 'itemCode', width: 260, render: (v: string) => <strong>{v}</strong> },
        { title: 'Base Code', dataIndex: 'baseCode', width: 260 },
        {
          title: '状态',
          dataIndex: 'orphan',
          width: 130,
          render: (v: boolean) => (v ? <Tag color="orange">孤儿 Base</Tag> : <Tag color="green">已匹配</Tag>),
        },
        updated,
        ops,
      ];
    }
    if (tab === 'base') {
      return [
        { title: 'Base Code', dataIndex: 'baseCode', width: 220, render: (v: string) => <strong>{v}</strong> },
        { title: 'Manu Planning Code', dataIndex: 'manuCode', width: 220 },
        { title: 'SAP Code', dataIndex: 'sapCode', width: 180 },
        {
          title: 'Category',
          dataIndex: 'category',
          width: 180,
          render: (v: string) => <CategoryTag category={v} />,
        },
        { title: 'Item 引用', dataIndex: 'refCount', width: 90, align: 'right' },
        updated,
        ops,
      ];
    }
    return [
      { title: 'SAP Code', dataIndex: 'sapCode', width: 240, render: (v: string) => <strong>{v}</strong> },
      { title: 'Category', dataIndex: 'category', width: 280, render: (v: string) => <CategoryTag category={v} /> },
      { title: 'Base 引用', dataIndex: 'refCount', width: 100, align: 'right' },
      {
        title: '状态',
        dataIndex: 'unused',
        width: 120,
        render: (v: boolean) => (v ? <Tag>未使用</Tag> : <Tag color="green">使用中</Tag>),
      },
      updated,
      ops,
    ];
  }, [tab, actions]);

  const stats = overview
    ? [
        { label: 'Item', value: overview.itemCount, hint: 'Item → Base 行数' },
        { label: 'Base', value: overview.baseCount, hint: 'Base → SAP 行数' },
        { label: 'SAP', value: overview.sapCount, hint: 'Base 表中的 SAP 编码' },
        { label: '分类', value: overview.categoryCount, hint: 'SAP → Category 分类数' },
        { label: '未分类', value: overview.uncategorized, hint: '正常状态，不阻断导入', tone: JNJ.warning },
        { label: '孤儿 Base', value: overview.orphanCount, hint: 'Item 引用了不存在的 Base', tone: JNJ.danger },
        { label: '未使用分类', value: overview.unusedCategories, hint: '分类表中没有 Base 引用', tone: JNJ.warning },
      ]
    : [];

  const categoryOptions = (overview?.categories ?? []).map((c) => ({ value: c }));

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column', background: JNJ.bg }}>
      <header
        style={{
          flex: '0 0 auto',
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '10px 20px',
          background: JNJ.surface,
          borderBottom: `1px solid ${JNJ.border}`,
        }}
      >
        <Button icon={<ArrowLeft size={16} />} onClick={onClose} aria-label="返回数据工作台">
          数据工作台
        </Button>
        <div style={{ width: 1, height: 24, background: JNJ.border, margin: '0 4px' }} />
        <BookOpen size={20} color={JNJ.red} />
        <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.3 }}>
          <span style={{ fontSize: 16, fontWeight: 600, color: JNJ.ink }}>主数据</span>
          <span style={{ fontSize: 12, color: JNJ.textMuted }}>
            代码匹配与产品分类 · 本机维护，历史周不回溯更新
          </span>
        </div>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
          <Button icon={<RefreshCw size={15} />} onClick={() => void refresh()} loading={loading}>
            刷新
          </Button>
          <Button
            type="primary"
            icon={<Upload size={15} />}
            loading={importing}
            onClick={() => setConfirmImport(true)}
            aria-label="导入匹配工作簿"
          >
            导入匹配工作簿
          </Button>
        </div>
      </header>

      <main style={{ flex: '1 1 auto', minHeight: 0, overflow: 'auto', padding: '16px 20px 28px' }}>
        <div style={{ maxWidth: 1500, margin: '0 auto' }}>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(145px, 1fr))',
              gap: 10,
              marginBottom: 12,
            }}
          >
            {stats.map((s) => (
              <div
                key={s.label}
                style={{
                  background: JNJ.surface,
                  border: `1px solid ${JNJ.border}`,
                  borderRadius: 6,
                  padding: '10px 12px',
                }}
              >
                <div style={{ fontSize: 12, color: JNJ.textMuted }}>{s.label}</div>
                <div style={{ fontSize: 22, fontWeight: 600, color: s.tone ?? JNJ.ink, fontVariantNumeric: 'tabular-nums' }}>
                  {s.value.toLocaleString()}
                </div>
                <div style={{ fontSize: 11, color: JNJ.textMuted }}>{s.hint}</div>
              </div>
            ))}
          </div>

          <div style={{ background: JNJ.surface, border: `1px solid ${JNJ.border}`, borderRadius: 6 }}>
            <Tabs
              activeKey={tab}
              onChange={(k) => changeTab(k as TabKey)}
              style={{ padding: '0 14px' }}
              items={[
                { key: 'item', label: 'Item → Base' },
                { key: 'base', label: 'Base → SAP' },
                { key: 'category', label: 'SAP → Category' },
                { key: 'price', label: '产品价格（预留）' },
              ]}
            />

            {tab === 'price' ? (
              <div style={{ padding: '40px 20px 56px' }}>
                <Empty
                  image={<BookOpen size={40} color={JNJ.textMuted} />}
                  description={
                    <div style={{ color: JNJ.text, maxWidth: 560, margin: '0 auto', lineHeight: 1.8 }}>
                      <div style={{ fontWeight: 600, marginBottom: 4 }}>产品价格工作簿将在后续版本加入</div>
                      <div style={{ color: JNJ.textMuted, fontSize: 12 }}>
                        价格通常还需要金额精度、币种、单位与生效日期，因此单独维护，不与代码匹配表混用。
                        当前页面位置已保留，不影响 Item / Base / SAP 分类的维护与导入。
                      </div>
                    </div>
                  }
                />
              </div>
            ) : (
              <div style={{ padding: '0 14px 14px' }}>
                {error && (
                  <Alert
                    type="error"
                    showIcon
                    closable
                    title={error}
                    onClose={() => setError('')}
                    style={{ marginBottom: 10 }}
                  />
                )}
                {notice && (
                  <Alert
                    type="success"
                    showIcon
                    closable
                    title={notice}
                    onClose={() => setNotice('')}
                    style={{ marginBottom: 10 }}
                  />
                )}
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '2px 0 10px', flexWrap: 'wrap' }}>
                  <Input.Search
                    key={tab}
                    allowClear
                    placeholder="搜索编码或分类"
                    defaultValue={search}
                    onSearch={(v) => {
                      setPage(1);
                      setSearch(v.trim());
                    }}
                    style={{ width: 280 }}
                  />
                  <Select
                    value={filter}
                    options={filterOptions}
                    onChange={(v) => {
                      setPage(1);
                      setFilter(v);
                    }}
                    style={{ width: 150 }}
                    aria-label="筛选主数据"
                  />
                  <span style={{ color: JNJ.textMuted, fontSize: 12 }}>
                    共 {total.toLocaleString()} 条 · 未分类是正常状态
                  </span>
                  <Button
                    type="primary"
                    icon={<Plus size={15} />}
                    onClick={openNew}
                    style={{ marginLeft: 'auto' }}
                  >
                    新增
                  </Button>
                </div>
                <Table<MasterRow>
                  size="small"
                  rowKey="key"
                  loading={loading}
                  columns={columns}
                  dataSource={rows}
                  scroll={{ x: 900 }}
                  pagination={{
                    current: page,
                    pageSize: PAGE_SIZE,
                    total,
                    showSizeChanger: false,
                    showTotal: (n) => `共 ${n.toLocaleString()} 条`,
                    onChange: (p) => setPage(p),
                  }}
                  locale={{ emptyText: <Empty description="没有匹配记录" /> }}
                />
              </div>
            )}
          </div>

          <Typography.Text style={{ display: 'block', marginTop: 10, color: JNJ.textMuted, fontSize: 12 }}>
            这些映射只影响之后导入/整合的数据，不会改写历史周；未分类、孤儿 Base 与未使用分类都允许存在。
            {overview?.lastImportAt ? ` · 上次导入 ${overview.lastImportAt}` : ' · 尚未导入匹配工作簿'}
          </Typography.Text>
        </div>
      </main>

      <Drawer
        title={editor?.mode === 'edit' ? `编辑 ${activeLabel}` : `新增 ${activeLabel}`}
        size={440}
        open={Boolean(editor)}
        onClose={() => setEditor(null)}
        extra={
          <Space>
            <Button onClick={() => setEditor(null)}>取消</Button>
            <Button type="primary" loading={saving} onClick={() => void save()}>
              保存
            </Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical" onFinish={() => void save()}>
          {tab === 'item' && (
            <>
              <Form.Item
                label="Item Code"
                name="itemCode"
                rules={[{ required: true, message: '请输入 Item Code' }]}
              >
                <Input disabled={editor?.mode === 'edit'} autoFocus />
              </Form.Item>
              <Form.Item
                label="Base Code"
                name="baseCode"
                rules={[{ required: true, message: '请输入 Base Code' }]}
              >
                <Input />
              </Form.Item>
            </>
          )}
          {tab === 'base' && (
            <>
              <Form.Item
                label="Base Code"
                name="baseCode"
                rules={[{ required: true, message: '请输入 Base Code' }]}
              >
                <Input disabled={editor?.mode === 'edit'} autoFocus />
              </Form.Item>
              <Form.Item label="Manu Planning Code" name="manuCode">
                <Input />
              </Form.Item>
              <Form.Item label="SAP Code" name="sapCode">
                <Input />
              </Form.Item>
            </>
          )}
          {tab === 'category' && (
            <>
              <Form.Item
                label="SAP Code"
                name="sapCode"
                rules={[{ required: true, message: '请输入 SAP Code' }]}
              >
                <Input disabled={editor?.mode === 'edit'} autoFocus />
              </Form.Item>
              <Form.Item
                label="Category"
                name="category"
                extra="留空表示未分类；未分类是正常状态，不会阻断导入。"
              >
                <AutoComplete
                  options={categoryOptions}
                  placeholder="选择已有分类，或输入新分类"
                  filterOption={(input, option) =>
                    String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
                  }
                />
              </Form.Item>
            </>
          )}
        </Form>
      </Drawer>

      <Modal
        open={confirmImport}
        title="导入匹配工作簿并覆盖主数据？"
        okText="选择工作簿"
        cancelText="取消"
        onOk={() => void runImport()}
        onCancel={() => setConfirmImport(false)}
        confirmLoading={importing}
      >
        <div style={{ lineHeight: 1.9 }}>
          <div>会一次性覆盖 Item → Base、Base → SAP、SAP → Category 三张表。</div>
          <div style={{ color: JNJ.textMuted, fontSize: 12 }}>
            工作簿中的 D–Q 分析/公式列不会导入；重复主键会去重。未分类、孤儿 Base 只提示，不会阻断。
          </div>
        </div>
      </Modal>

      <Modal
        open={Boolean(importResult)}
        title="主数据导入完成"
        onCancel={() => setImportResult(null)}
        footer={
          <Button type="primary" onClick={() => setImportResult(null)}>
            知道了
          </Button>
        }
      >
        {importResult && (
          <>
            <Descriptions column={2} size="small" bordered>
              <Descriptions.Item label="Item">{importResult.items.toLocaleString()}</Descriptions.Item>
              <Descriptions.Item label="Base">{importResult.bases.toLocaleString()}</Descriptions.Item>
              <Descriptions.Item label="SAP 分类">{importResult.categories.toLocaleString()}</Descriptions.Item>
              <Descriptions.Item label="重复行">{importResult.duplicateRows.toLocaleString()}</Descriptions.Item>
              <Descriptions.Item label="未分类">{importResult.uncategorized.toLocaleString()}</Descriptions.Item>
              <Descriptions.Item label="孤儿 Base">{importResult.orphans.toLocaleString()}</Descriptions.Item>
            </Descriptions>
            <div style={{ marginTop: 10, color: JNJ.textMuted, fontSize: 12 }}>
              来源：{importResult.name} · {importResult.importedAt || '刚刚'}
            </div>
            {importResult.warnings.length > 0 && (
              <Alert
                type="warning"
                showIcon
                style={{ marginTop: 12 }}
                title={`有 ${importResult.warnings.length} 条数据提示`}
                description={
                  <ul style={{ margin: 0, paddingLeft: 18, maxHeight: 180, overflow: 'auto' }}>
                    {importResult.warnings.slice(0, 12).map((w, i) => (
                      <li key={`${i}-${w}`}>{w}</li>
                    ))}
                    {importResult.warnings.length > 12 && <li>… 其余请查看日志</li>}
                  </ul>
                }
              />
            )}
          </>
        )}
      </Modal>
    </div>
  );
}
