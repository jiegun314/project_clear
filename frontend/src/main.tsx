import React from 'react';
import { createRoot } from 'react-dom/client';
import { App as AntApp, ConfigProvider, theme as antdTheme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import App from './App';
import { theme, JNJ } from './theme/jnj';
import { ErrorBoundary, installGlobalErrorReporting } from './components/ErrorBoundary';
import './style.css';

installGlobalErrorReporting();

const container = document.getElementById('root')!;

// A theme object that antd rejects would blank the whole tree, so the tokens
// are built defensively and any failure is reported instead of swallowed.
let activeTheme = theme;
try {
  void antdTheme.getDesignToken({ ...theme.token, token: theme.token });
} catch (e) {
  console.error('[CLEAR] theme token rejected by antd', e);
  activeTheme = { ...theme, components: undefined };
}

// The boundary sits outside ConfigProvider on purpose: a throw while antd
// derives its theme would otherwise unmount the whole root and leave the
// window blank, with nothing left to report the failure.
createRoot(container).render(
  <React.StrictMode>
    <ErrorBoundary>
      <ConfigProvider locale={zhCN} theme={activeTheme}>
        <AntApp>
          <App />
        </AntApp>
      </ConfigProvider>
    </ErrorBoundary>
  </React.StrictMode>,
);

(window as unknown as Record<string, unknown>).__JNJ__ = JNJ;
