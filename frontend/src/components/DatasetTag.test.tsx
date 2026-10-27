import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DatasetTag } from './DatasetTag';

describe('DatasetTag', () => {
  it('renders 整合 / Global / Local in identical centered boxes', () => {
    render(
      <>
        <DatasetTag dataset="merged" />
        <DatasetTag dataset="global" />
        <DatasetTag dataset="local" />
      </>,
    );

    for (const label of ['整合', 'Global', 'Local']) {
      const el = screen.getByText(label);
      expect(el.style.width).toBe('45px');
      expect(el.style.height).toBe('20px');
      expect(el.style.display).toBe('inline-flex');
      expect(el.style.alignItems).toBe('center');
      expect(el.style.justifyContent).toBe('center');
    }
  });
});
