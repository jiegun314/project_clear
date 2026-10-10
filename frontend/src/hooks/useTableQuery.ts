import { useCallback, useEffect, useRef, useState } from 'react';
import type { TableProps } from 'antd';
import { api } from '../services/api';
import type { GridHeader, GridRow } from '../types';

/** Which column the backend is sorting by, and which way. */
export interface SortState {
  field: string;
  desc: boolean;
}

export interface TableQueryOptions {
  /**
   * What to read. `null` means nothing is selected: no request is made and the
   * header is cleared. The main grid uses `''` for the staging area, which is a
   * selection like any other.
   */
  source: string | null;
  /** The page size the caller starts from, and the one it is reset to when this changes. */
  defaultPageSize: number;
  /** Bumped by the caller to force a reload without changing anything else. */
  reloadToken?: number;
  /**
   * Whether a failed query clears the rows.
   *
   * The main grid clears them: leaving the previous week's rows under a new
   * week's header would be a lie. The archive window keeps them, which is how it
   * has always behaved.
   */
  clearRowsOnError?: boolean;
  /**
   * Called once per source, as soon as the first request has settled either way.
   * The archive window uses it to drop the full-screen cover it puts up the
   * moment the user clicks 查看.
   */
  onSettled?: () => void;
}

export interface TableQuery {
  header: GridHeader | null;
  rows: GridRow[];
  total: number;
  page: number;
  pageSize: number;
  search: string;
  setSearch: (value: string) => void;
  sort: SortState;
  loading: boolean;
  setPage: (page: number) => void;
  /** Changes the page size and goes back to the first page. */
  changePageSize: (size: number) => void;
  reload: () => void;
  onTableChange: NonNullable<TableProps<GridRow>['onChange']>;
}

/**
 * The state machine behind a data table: one header request per source, one
 * query per (source, page, page size, search, sort), and a guard that stops a
 * slow earlier answer from overwriting a newer one.
 *
 * Both tables in the application are this machine; they differ in the options
 * above and in how they are laid out, not in how they read data.
 */
export function useTableQuery(opt: TableQueryOptions): TableQuery {
  const { source, defaultPageSize, reloadToken = 0, clearRowsOnError = false } = opt;

  const [header, setHeader] = useState<GridHeader | null>(null);
  const [rows, setRows] = useState<GridRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(defaultPageSize);
  const [search, setSearch] = useState('');
  const [debounced, setDebounced] = useState('');
  const [sort, setSort] = useState<SortState>({ field: 'seq', desc: false });
  const [loading, setLoading] = useState(false);
  const reqId = useRef(0);

  // The caller usually hands in an inline arrow, so only the latest one is kept
  // and nothing depends on its identity — depending on it would re-issue the
  // header request on every parent render.
  const onSettledRef = useRef(opt.onSettled);
  useEffect(() => {
    onSettledRef.current = opt.onSettled;
  }, [opt.onSettled]);

  // Which source has already been announced, so switching weeks announces again
  // while paging and sorting do not.
  const settledFor = useRef<string | null>(null);
  const notifySettled = useCallback((forSource: string | null) => {
    if (settledFor.current === forSource) return;
    settledFor.current = forSource;
    onSettledRef.current?.();
  }, []);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search.trim()), 250);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    setPage(1);
  }, [source, debounced, sort.field, sort.desc, reloadToken]);

  useEffect(() => {
    setPageSize(defaultPageSize);
  }, [defaultPageSize]);

  useEffect(() => {
    if (source === null) {
      setHeader(null);
      return;
    }
    let alive = true;
    settledFor.current = null;
    api
      .getGridHeader(source)
      .then((h) => {
        if (alive) setHeader(h);
      })
      .catch(() => {
        if (!alive) return;
        setHeader(null);
        // Without a header there is no query to come, so nothing else would
        // settle and the caller's cover would spin forever.
        notifySettled(source);
      });
    return () => {
      alive = false;
    };
    // reloadToken belongs here as well as in the page reset: after an import the
    // staging area gains its week columns, and the header fetched while it was
    // empty would leave the grid with no columns and no rows to show.
  }, [source, notifySettled, reloadToken]);

  const load = useCallback(async () => {
    if (source === null || !header) return;
    const id = ++reqId.current;
    setLoading(true);
    try {
      const res = await api.queryData({
        source,
        page,
        pageSize,
        search: debounced,
        sortField: sort.field,
        sortDesc: sort.desc,
        filters: {},
      });
      // A slower earlier request must not overwrite a newer one.
      if (id !== reqId.current) return;
      setRows(res.rows ?? []);
      setTotal(res.total ?? 0);
    } catch {
      // Always caught: an unhandled rejection here would be reported as a
      // crash of the whole window rather than as a failed read.
      if (id === reqId.current && clearRowsOnError) {
        setRows([]);
        setTotal(0);
      }
    } finally {
      if (id === reqId.current) {
        setLoading(false);
        notifySettled(source);
      }
    }
  }, [source, header, page, pageSize, debounced, sort, notifySettled, clearRowsOnError]);

  useEffect(() => {
    // reloadToken is the parent's "look again" signal: the query itself does not
    // read it, so it is named here to make this a deliberate trigger — the shell
    // bumps it after an import, a commit or a clear.
    void reloadToken;
    void load();
  }, [load, reloadToken]);

  const changePageSize = useCallback((size: number) => {
    setPageSize(size);
    setPage(1);
  }, []);

  // Sorting and paging are both backend concerns: the columns only declare that
  // they are sortable and report the intent.
  const onTableChange = useCallback<NonNullable<TableProps<GridRow>['onChange']>>(
    (pagination, _filters, sorter) => {
      const s = Array.isArray(sorter) ? sorter[0] : sorter;
      const key = String(s?.columnKey ?? '');
      setSort(key && s?.order ? { field: key, desc: s.order === 'descend' } : { field: 'seq', desc: false });
      // The main grid has no table pagination of its own, so this stays undefined
      // there and its separate footer owns the page.
      if (pagination?.current) setPage(pagination.current);
    },
    [],
  );

  return {
    header,
    rows,
    total,
    page,
    pageSize,
    search,
    setSearch,
    sort,
    loading,
    setPage,
    changePageSize,
    reload: () => void load(),
    onTableChange,
  };
}
