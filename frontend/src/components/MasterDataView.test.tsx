import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MasterDataView } from './MasterDataView';
import { api } from '../services/api';
import type { MasterImportResult, MasterOverview, MasterPage, MasterQuery, PriceImportResult } from '../types';

vi.mock('../services/api', () => ({
  api: {
    getMasterOverview: vi.fn(),
    queryMasterData: vi.fn(),
    saveMasterRow: vi.fn(),
    deleteMasterRow: vi.fn(),
    importMasterWorkbook: vi.fn(),
    importPriceWorkbook: vi.fn(),
  },
}));

const overview: MasterOverview = {
  itemCount: 5224,
  baseCount: 5854,
  sapCount: 1756,
  categoryCount: 33,
  uncategorized: 1504,
  orphanCount: 161,
  unusedCategories: 38,
  priceCount: 94502,
  priceMissing: 1024,
  categories: ['Cat A', 'Cat B'],
  lastImportAt: '2026-11-18 22:00:00',
  lastPriceImportAt: '2026-11-19 21:30:00',
};

const itemPage: MasterPage = {
  rows: [
    { table: 'item', key: 'I-1', itemCode: 'I-1', baseCode: 'B-1', orphan: false, updatedAt: '2026-11-18' },
    { table: 'item', key: 'I-2', itemCode: 'I-2', baseCode: 'B-MISSING', orphan: true, updatedAt: '2026-11-18' },
  ],
  total: 2,
  page: 1,
  pageSize: 50,
};

const basePage: MasterPage = {
  rows: [
    {
      table: 'base',
      key: 'B-1',
      baseCode: 'B-1',
      manuCode: 'M-1',
      sapCode: 'S-1',
      category: 'Cat A',
      refCount: 7,
      uncategorized: false,
      updatedAt: '2026-11-18',
    },
    {
      table: 'base',
      key: 'B-2',
      baseCode: 'B-2',
      manuCode: 'M-2',
      sapCode: 'S-2',
      category: '',
      refCount: 0,
      uncategorized: true,
      updatedAt: '2026-11-18',
    },
  ],
  total: 2,
  page: 1,
  pageSize: 50,
};

const pricePage: MasterPage = {
  rows: [
    { table: 'price', key: '000.009', material: '000.009', standardCost: 107.36, updatedAt: '2026-11-19' },
    { table: 'price', key: '000.010', material: '000.010', standardCost: null, updatedAt: '2026-11-19' },
    { table: 'price', key: '000.011', material: '000.011', standardCost: 0, updatedAt: '2026-11-19' },
  ],
  total: 3,
  page: 1,
  pageSize: 50,
};

const categoryPage: MasterPage = {
  rows: [
    { table: 'category', key: 'S-1', sapCode: 'S-1', category: 'Cat A', refCount: 3, unused: false },
    { table: 'category', key: 'S-UNUSED', sapCode: 'S-UNUSED', category: '', refCount: 0, unused: true },
  ],
  total: 2,
  page: 1,
  pageSize: 50,
};

function pageFor(q: MasterQuery): MasterPage {
  if (q.table === 'base') return basePage;
  if (q.table === 'category') return categoryPage;
  if (q.table === 'price') return pricePage;
  return itemPage;
}

describe('MasterDataView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getMasterOverview).mockResolvedValue(overview);
    vi.mocked(api.queryMasterData).mockImplementation(async (q) => pageFor(q));
    vi.mocked(api.saveMasterRow).mockResolvedValue();
    vi.mocked(api.deleteMasterRow).mockResolvedValue();
    vi.mocked(api.importMasterWorkbook).mockResolvedValue(null);
    vi.mocked(api.importPriceWorkbook).mockResolvedValue(null);
  });

  it('shows the master counters and the item table', async () => {
    render(<MasterDataView onClose={() => undefined} />);
    expect((await screen.findAllByText('I-1')).length).toBeGreaterThan(0);
    expect(screen.getByText('5,224')).toBeTruthy();
    expect(screen.getByText('5,854')).toBeTruthy();
    expect(screen.getByText('1,504')).toBeTruthy();
    expect(screen.getByText('161')).toBeTruthy();
    expect(screen.getAllByText('孤儿 Base').length).toBeGreaterThan(0);
    expect(api.queryMasterData).toHaveBeenCalledWith(
      expect.objectContaining({ table: 'item', page: 1, pageSize: 50, filter: 'all' }),
    );
  });

  it('switches tables and shows category state', async () => {
    render(<MasterDataView onClose={() => undefined} />);
    await screen.findAllByText('I-1');

    fireEvent.click(screen.getAllByRole('tab', { name: 'Base → SAP' })[0]);
    expect(await screen.findByText('B-1')).toBeTruthy();
    expect(screen.getAllByText('未分类').length).toBeGreaterThan(0);
    expect(api.queryMasterData).toHaveBeenCalledWith(
      expect.objectContaining({ table: 'base', page: 1 }),
    );

    fireEvent.click(screen.getAllByRole('tab', { name: 'SAP → Category' })[0]);
    expect(await screen.findByText('S-UNUSED')).toBeTruthy();
    expect(screen.getByText('未使用')).toBeTruthy();
  });

  it('shows the real price table and price counters', async () => {
    render(<MasterDataView onClose={() => undefined} />);
    await screen.findAllByText('I-1');
    fireEvent.click(screen.getAllByRole('tab', { name: '产品价格' })[0]);
    expect(await screen.findByText('107.36')).toBeTruthy();
    expect(screen.getAllByText('未定价').length).toBeGreaterThan(0);
    expect(screen.getByText('94,502')).toBeTruthy();
    expect(api.queryMasterData).toHaveBeenCalledWith(
      expect.objectContaining({ table: 'price', page: 1, pageSize: 50 }),
    );
  });

  it('imports a price workbook after confirmation and reports the result', async () => {
    const result: PriceImportResult = {
      path: '/tmp/price.xlsx',
      name: 'price.xlsx',
      prices: 94502,
      duplicateRows: 23,
      missingCosts: 1024,
      skippedRows: 0,
      warnings: [],
      importedAt: '2026-11-19 21:40:00',
    };
    vi.mocked(api.importPriceWorkbook).mockResolvedValue(result);
    render(<MasterDataView onClose={() => undefined} />);
    await screen.findAllByText('I-1');
    fireEvent.click(screen.getAllByRole('tab', { name: '产品价格' })[0]);
    await screen.findByText('000.009');

    fireEvent.click(screen.getByRole('button', { name: '导入价格工作簿' }));
    expect(screen.getByText('导入价格工作簿并覆盖价格主数据？')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: '选择工作簿' }));

    expect(await screen.findByText('产品价格导入完成')).toBeTruthy();
    expect(api.importPriceWorkbook).toHaveBeenCalledTimes(1);
    expect(screen.getByText('price.xlsx', { exact: false })).toBeTruthy();
    expect(screen.getAllByText('1,024').length).toBeGreaterThan(0);
  });

  it('imports a workbook after confirmation and reports the result', async () => {
    const result: MasterImportResult = {
      path: '/tmp/match.xlsx',
      name: 'match.xlsx',
      items: 5224,
      bases: 5854,
      categories: 290,
      duplicateRows: 46,
      orphans: 161,
      uncategorized: 1504,
      warnings: ['Base B-1 对应多组 SAP/Manu 值'],
      importedAt: '2026-11-18 22:30:00',
    };
    vi.mocked(api.importMasterWorkbook).mockResolvedValue(result);
    render(<MasterDataView onClose={() => undefined} />);
    await screen.findAllByText('I-1');

    fireEvent.click(screen.getByRole('button', { name: '导入匹配工作簿' }));
    expect(screen.getByText('导入匹配工作簿并覆盖主数据？')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: '选择工作簿' }));

    expect(await screen.findByText('主数据导入完成')).toBeTruthy();
    expect(api.importMasterWorkbook).toHaveBeenCalledTimes(1);
    expect(screen.getByText('match.xlsx', { exact: false })).toBeTruthy();
    expect(screen.getByText('Base B-1 对应多组 SAP/Manu 值')).toBeTruthy();
  });

  it('closes the page from the header', async () => {
    const onClose = vi.fn();
    render(<MasterDataView onClose={onClose} />);
    await screen.findAllByText('I-1');
    fireEvent.click(screen.getByRole('button', { name: '返回数据工作台' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });
});
