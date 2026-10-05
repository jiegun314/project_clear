import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// jsdom implements neither ResizeObserver nor matchMedia, and antd's Table uses
// both on mount (it measures its header and watches the viewport). Without these
// the component tests fail in setup rather than in the behaviour under test.

if (!('ResizeObserver' in globalThis)) {
  class ResizeObserverStub {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  });
}

if (typeof window !== 'undefined' && !window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

// jsdom has no layout engine: every element reports a height and width of zero,
// so any behaviour that depends on measurement (the resizable split, the table's
// own height) is untestable without a size. These fixed values are what the
// split's bounds are derived from in App.test.tsx; a test that needs a different
// measurement stubs its own element.
//
// 300 / 800 mirror a plausible window: the lower half starts at 300px and the
// container is 800px tall, so the bounds are 240..360 and the ceiling the grid
// imposes (container - TOP_MIN_HEIGHT - SPLIT_BAR = 592) does not bind.
Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
  configurable: true,
  get: () => 300,
});
Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
  configurable: true,
  get: () => 800,
});

// Each test starts from an empty document, so a rendered grid cannot be found by
// the next test's queries.
afterEach(cleanup);

// antd reports a deprecated prop through console.error and carries on, which is
// how a codebase quietly stays on an API that is about to be removed: four of
// these were sitting in the components until the shell was first rendered under
// test. Nothing else in these tests logs through console.error, so any
// deprecation antd raises is treated as a failure of the test that caused it.
//
// They are collected rather than thrown where they happen: throwing inside a
// render turns into a component error, which an error boundary would hide.
const ANTD_DEPRECATION = /\[antd: [^\]]+\] `[^`]+` is deprecated/;
const deprecations: string[] = [];
const realConsoleError = console.error;

console.error = (...args: unknown[]) => {
  const text = args.map((a) => (typeof a === 'string' ? a : String(a))).join(' ');
  if (ANTD_DEPRECATION.test(text)) deprecations.push(text);
  realConsoleError(...args);
};

afterEach(() => {
  const raised = deprecations.splice(0);
  if (raised.length > 0) {
    throw new Error(
      `antd reported ${raised.length} deprecated prop(s) while rendering:\n  ` +
        raised.join('\n  '),
    );
  }
});
