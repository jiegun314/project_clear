import { useCallback, useEffect, useRef, useState } from 'react';
import { onEvent } from '../services/api';
import { acceptProgress, finishedProgress, type TaskProgress } from '../lib/taskProgress';

/** A line shown above the grid after an action: success, failure, or plain news. */
export interface Notice {
  type: 'success' | 'error' | 'info';
  text: string;
}

export interface TaskRunner {
  busy: boolean;
  progress: TaskProgress | null;
  notice: Notice | null;
  setNotice: (n: Notice | null) => void;
  /** Starts a task: from here the status bar shows progress instead of a tick. */
  begin: (stage: string, done: number, total: number) => void;
  /** Ends a task: a green tick when it worked, nothing at all when it did not. */
  finish: (ok: boolean) => void;
}

/**
 * What the shell shows while a long task runs: the busy flag, the progress line
 * in the status bar, and the notice above the grid.
 *
 * The backend keeps sending progress after a task is over (it reports the last
 * step a moment after the await resolves), which is why the latest one is only
 * accepted while a task is actually running — otherwise the bar would sit on a
 * red "running" forever.
 */
export function useTaskRunner(): TaskRunner {
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<TaskProgress | null>(null);
  const [notice, setNotice] = useState<Notice | null>(null);
  const taskRunning = useRef(false);

  useEffect(() => {
    const offProgress = onEvent<TaskProgress>('task:progress', (p) => {
      const accepted = acceptProgress(taskRunning.current, p);
      if (accepted) setProgress(accepted);
    });
    const offDone = onEvent<{ ok: boolean; message?: string }>('task:done', (d) => {
      if (d && !d.ok && d.message) setNotice({ type: 'error', text: d.message });
    });
    return () => {
      offProgress();
      offDone();
    };
  }, []);

  const begin = useCallback((stage: string, done: number, total: number) => {
    setBusy(true);
    taskRunning.current = true;
    setProgress({ stage, done, total });
  }, []);

  const finish = useCallback((ok: boolean) => {
    taskRunning.current = false;
    setBusy(false);
    setProgress(ok ? finishedProgress() : null);
  }, []);

  return { busy, progress, notice, setNotice, begin, finish };
}
