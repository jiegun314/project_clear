import { useCallback, useEffect, useState } from 'react';
import { api } from '../services/api';
import type { AppConfig, StagingFilesView, Status } from '../types';

/** Used until the backend answers, so the first paint has something to render. */
const DEFAULT_CONFIG: AppConfig = {
  configVersion: 2,
  readColumns: 20,
  locFilter: 'WH_CNB',
  pageSize: 200,
  headerDisplay: 'twoRow',
  exportMode: 'clean',
  exportDir: '',
};

export interface BackendState {
  cfg: AppConfig;
  status: Status | null;
  /** The 整合清单, behind the toolbar's 已导入文件 button. */
  stagedFiles: StagingFilesView | null;
  /** Bumped to make the grids reload without re-reading the status. */
  reloadToken: number;
  refresh: () => Promise<void>;
  /** After anything that changed the data: reload the grids and the status bar. */
  afterChange: () => Promise<void>;
  applyConfig: (c: AppConfig) => void;
}

/**
 * The three things the shell reads from the backend on every change: the
 * parameters, the status bar, and the current 整合清单.
 *
 * `refresh` deliberately swallows errors: the webview can paint before the
 * backend has finished starting, and the next refresh picks it up.
 */
export function useBackendState(): BackendState {
  const [cfg, setCfg] = useState<AppConfig>(DEFAULT_CONFIG);
  const [status, setStatus] = useState<Status | null>(null);
  const [stagedFiles, setStagedFiles] = useState<StagingFilesView | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  const refresh = useCallback(async () => {
    try {
      const [s, c, files] = await Promise.all([
        api.getStatus(),
        api.getConfig(),
        api.getStagingFiles(),
      ]);
      setStatus(s);
      setCfg(c.config);
      setStagedFiles(files);
    } catch {
      /* the shell may not be ready yet; the first poll will pick it up */
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const afterChange = useCallback(async () => {
    setReloadToken((t) => t + 1);
    await refresh();
  }, [refresh]);

  return {
    cfg,
    status,
    stagedFiles,
    reloadToken,
    refresh,
    afterChange,
    applyConfig: setCfg,
  };
}
