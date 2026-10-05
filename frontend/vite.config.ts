import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// One config for both the build and the tests: vitest reads the same plugins and
// resolution as `vite build`, so a test cannot pass against a module graph the
// application never has.
export default defineConfig({
  plugins: [react()],
  test: {
    // The components are browser code: antd's Table measures itself on mount and
    // the grid listens for resize, neither of which exists in plain node.
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    // A mock that leaks from one test into the next is the classic way to end up
    // with a test that passes for the wrong reason.
    restoreMocks: true,
  },
});
