import React from 'react';
import { Alert, Typography, Button } from 'antd';
import { hasBackend, api } from '../services/api';

interface Props {
  children: React.ReactNode;
}

interface State {
  error: Error | null;
  info: string;
}

/**
 * A render crash inside the webview used to present as a blank white window,
 * which is impossible to diagnose from the outside. The boundary keeps the
 * window alive, shows what failed, and forwards the detail to the Go log.
 */
export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null, info: '' };

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    const stack = `${error.message}\n${info.componentStack ?? ''}`;
    this.setState({ info: info.componentStack ?? '' });
    if (hasBackend()) {
      // Best effort: never let reporting itself throw.
      try {
        void (api as unknown as {
          ReportFrontendError?: (m: string, s: string, src: string) => Promise<void>;
        }).ReportFrontendError?.(error.message, stack, '界面');
      } catch {
        /* ignore */
      }
    }
    console.error('[CLEAR] render failed', error, info);
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div style={{ padding: 32 }}>
        <Alert
          type="error"
          showIcon
          title="界面渲染失败"
          description={
            <>
              <Typography.Paragraph style={{ marginBottom: 8 }}>
                {this.state.error.message}
              </Typography.Paragraph>
              <Typography.Paragraph type="secondary" style={{ fontSize: 12, whiteSpace: 'pre-wrap' }}>
                {this.state.info?.trim()}
              </Typography.Paragraph>
              <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
                详细信息已写入 CLEAR 运行日志（数据目录下的 logs/）。
              </Typography.Paragraph>
            </>
          }
        />
        <Button style={{ marginTop: 16 }} onClick={() => window.location.reload()}>
          重新加载界面
        </Button>
      </div>
    );
  }
}

/** Reports errors that escape React, such as a failed async render. */
export function installGlobalErrorReporting() {
  if (typeof window === 'undefined') return;
  const send = (message: string, stack: string, source: string) => {
    console.error(`[CLEAR] ${message}`, stack);
    if (!hasBackend()) return;
    try {
      void (api as unknown as {
        ReportFrontendError?: (m: string, s: string, src: string) => Promise<void>;
      }).ReportFrontendError?.(message, stack, source);
    } catch {
      /* ignore */
    }
  };
  window.addEventListener('error', (e) => {
    send(e.message || String(e.error ?? 'unknown error'), (e.error as Error)?.stack ?? '', '全局');
  });
  window.addEventListener('unhandledrejection', (e) => {
    const r = e.reason as Error | undefined;
    send(r?.message || String(e.reason), r?.stack ?? '', '异步');
  });
}
