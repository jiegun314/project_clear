import React from 'react';
import { createRoot } from 'react-dom/client';
import { App as AntApp, ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import App from './App';
import { theme, JNJ } from './theme/jnj';
import './style.css';

const container = document.getElementById('root')!;

createRoot(container).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN} theme={theme}>
      <AntApp>
        <App />
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
);

// Expose the palette for debugging in the webview console.
(window as unknown as Record<string, unknown>).__JNJ__ = JNJ;
