import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import { useFilterUrl, type FilterConfig } from './useFilterUrl';

/**
 * Helper component that exercises useFilterUrl and exposes its state
 * in data-testid attributes so tests can inspect it.
 */
function TestComponent({ config }: { config: FilterConfig }) {
  const filter = useFilterUrl(config);
  return (
    <div>
      <div data-testid="activeCount">{filter.activeCount}</div>
      <div data-testid="values">{JSON.stringify(filter.values)}</div>
      {Object.keys(filter.values).map((key) => (
        <div key={key} data-testid={`value-${key}`}>
          {(filter.values as Record<string, string>)[key]}
        </div>
      ))}
      <button data-testid="setFilter-status" onClick={() => filter.setFilter('status', 'pending')}>
        Set Status
      </button>
      <button data-testid="setFilter-severity" onClick={() => filter.setFilter('severity', 'critical')}>
        Set Severity
      </button>
      <button data-testid="clearFilter-status" onClick={() => filter.clearFilter('status')}>
        Clear Status
      </button>
      <button data-testid="clearAll" onClick={() => filter.clearAll()}>
        Clear All
      </button>
    </div>
  );
}

/**
 * Helper that also exposes the current URL search string.
 */
function TestComponentWithLocation({ config }: { config: FilterConfig }) {
  const filter = useFilterUrl(config);
  const location = useLocation();
  return (
    <div>
      <div data-testid="search">{location.search}</div>
      <div data-testid="values">{JSON.stringify(filter.values)}</div>
      <button data-testid="setFilter-status" onClick={() => filter.setFilter('status', 'pending')}>
        Set Status
      </button>
      <button data-testid="clearAll" onClick={() => filter.clearAll()}>
        Clear All
      </button>
    </div>
  );
}

function renderWithRouter(ui: React.ReactElement, initialEntries = ['/']) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      {ui}
    </MemoryRouter>
  );
}

const config: FilterConfig = {
  status: { urlParam: 'status' },
  severity: { urlParam: 'severity', default: 'low' },
};

describe('useFilterUrl', () => {
  it('reads initial state from URL params on mount', () => {
    renderWithRouter(
      <TestComponent config={config} />,
      ['/?status=pending&severity=critical']
    );
    expect(screen.getByTestId('value-status')).toHaveTextContent('pending');
    expect(screen.getByTestId('value-severity')).toHaveTextContent('critical');
  });

  it('uses defaults when URL params are missing', () => {
    renderWithRouter(
      <TestComponent config={config} />,
      ['/']
    );
    expect(screen.getByTestId('value-status')).toHaveTextContent('');
    // severity defaults to 'low'
    expect(screen.getByTestId('value-severity')).toHaveTextContent('low');
  });

  it('returns zero activeCount when all values are defaults', () => {
    renderWithRouter(
      <TestComponent config={config} />,
      ['/']
    );
    expect(screen.getByTestId('activeCount')).toHaveTextContent('0');
  });

  it('returns correct activeCount when filters differ from defaults', () => {
    renderWithRouter(
      <TestComponent config={config} />,
      ['/?status=approved']
    );
    // status is non-default -> count 1, severity is default -> not counted
    expect(screen.getByTestId('activeCount')).toHaveTextContent('1');
  });

  it('setFilter changes a value', () => {
    renderWithRouter(<TestComponent config={config} />, ['/']);
    fireEvent.click(screen.getByTestId('setFilter-status'));
    expect(screen.getByTestId('value-status')).toHaveTextContent('pending');
  });

  it('setFilter increments activeCount', () => {
    renderWithRouter(<TestComponent config={config} />, ['/']);
    fireEvent.click(screen.getByTestId('setFilter-status'));
    expect(screen.getByTestId('activeCount')).toHaveTextContent('1');
  });

  it('setFilter increments activeCount for multiple filters', () => {
    renderWithRouter(<TestComponent config={config} />, ['/']);
    fireEvent.click(screen.getByTestId('setFilter-status'));
    fireEvent.click(screen.getByTestId('setFilter-severity'));
    expect(screen.getByTestId('activeCount')).toHaveTextContent('2');
  });

  it('clearFilter resets a single filter to default', () => {
    renderWithRouter(<TestComponent config={config} />, ['/?status=approved']);
    fireEvent.click(screen.getByTestId('clearFilter-status'));
    expect(screen.getByTestId('value-status')).toHaveTextContent('');
  });

  it('clearFilter decrements activeCount', () => {
    renderWithRouter(<TestComponent config={config} />, ['/?status=approved&severity=critical']);
    expect(screen.getByTestId('activeCount')).toHaveTextContent('2');
    fireEvent.click(screen.getByTestId('clearFilter-status'));
    expect(screen.getByTestId('activeCount')).toHaveTextContent('1');
  });

  it('clearAll resets all filters to defaults', () => {
    renderWithRouter(
      <TestComponent config={config} />,
      ['/?status=approved&severity=critical']
    );
    fireEvent.click(screen.getByTestId('clearAll'));
    expect(screen.getByTestId('value-status')).toHaveTextContent('');
    expect(screen.getByTestId('value-severity')).toHaveTextContent('low');
  });

  it('clearAll sets activeCount to 0', () => {
    renderWithRouter(
      <TestComponent config={config} />,
      ['/?status=approved&severity=critical']
    );
    fireEvent.click(screen.getByTestId('clearAll'));
    expect(screen.getByTestId('activeCount')).toHaveTextContent('0');
  });

  it('clearAll does not affect defaults-only state', () => {
    renderWithRouter(<TestComponent config={config} />, ['/']);
    fireEvent.click(screen.getByTestId('clearAll'));
    expect(screen.getByTestId('value-status')).toHaveTextContent('');
    expect(screen.getByTestId('value-severity')).toHaveTextContent('low');
    expect(screen.getByTestId('activeCount')).toHaveTextContent('0');
  });

  it('setFilter writes the value to the URL', () => {
    renderWithRouter(<TestComponentWithLocation config={config} />, ['/']);
    fireEvent.click(screen.getByTestId('setFilter-status'));
    expect(screen.getByTestId('search').textContent).toContain('status=pending');
  });

  it('clearAll removes filter params from the URL', () => {
    renderWithRouter(<TestComponentWithLocation config={config} />, ['/?status=pending&severity=critical']);
    fireEvent.click(screen.getByTestId('clearAll'));
    // Both non-default values cleared — URL should have no filter params
    const search = screen.getByTestId('search').textContent || '';
    expect(search).not.toContain('status=');
    expect(search).not.toContain('severity=');
  });
});

import { buildFilterPills } from './useFilterUrl';
import type { FilterUrlState } from './useFilterUrl';

describe('buildFilterPills', () => {
  it('returns pills for non-default values', () => {
    const filter = {
      values: { status: 'pending', severity: '' },
      clearFilter: () => {},
    } as unknown as FilterUrlState<typeof config>;
    const pills = buildFilterPills(filter, config);
    expect(pills).toHaveLength(1);
    expect(pills[0].label).toBe('status');
    expect(pills[0].value).toBe('pending');
  });

  it('uses config key as label fallback when label is not set', () => {
    const noLabelConfig = {
      foo: { urlParam: 'foo' },
    };
    const filter = {
      values: { foo: 'bar' },
      clearFilter: () => {},
    } as unknown as FilterUrlState<typeof noLabelConfig>;
    const pills = buildFilterPills(filter, noLabelConfig);
    expect(pills[0].label).toBe('foo');
  });

  it('skips default values', () => {
    const filter = {
      values: { status: '', severity: 'low' },
      clearFilter: () => {},
    } as unknown as FilterUrlState<typeof config>;
    const pills = buildFilterPills(filter, config);
    expect(pills).toHaveLength(0);
  });
});
