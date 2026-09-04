import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi } from 'vitest';

// Mock all API hooks before importing the page
vi.mock('../hooks/useUpdates', () => ({
  useUpdates: vi.fn(() => ({
    data: { updates: [], total: 0, stats: { pending_updates: 0, approved_updates: 0, critical_updates: 0, high_updates: 0 } },
    isPending: false,
    error: null,
  })),
  usePackages: vi.fn(() => ({
    data: { packages: [], total: 0 },
    isPending: false,
    error: null,
  })),
  useUpdate: vi.fn(() => ({ data: null })),
  usePackageFleet: vi.fn(() => ({ data: null })),
  usePackageVersions: vi.fn(() => ({ data: null })),
  useUpdateLifecycle: vi.fn(() => ({ data: null })),
  useApproveUpdate: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useRejectUpdate: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useInstallUpdate: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useApproveMultipleUpdates: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useRetryCommand: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useReopenUpdate: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useResolveUpdate: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useCancelCommand: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock('../hooks/useCommands', () => ({
  useRecentCommands: vi.fn(() => ({ data: { commands: [] } })),
}));

vi.mock('../hooks/useDebounce', () => ({
  useDebounce: vi.fn((value) => value),
}));

import Updates from './Updates';

function renderPage(initialEntries = ['/updates']) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <Updates />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('Updates page filter integration', () => {
  it('renders the page header', () => {
    renderPage();
    expect(screen.getByText('Updates')).toBeInTheDocument();
  });

  it('renders FilterBar with search input', () => {
    renderPage();
    expect(screen.getByPlaceholderText('Search updates by package name...')).toBeInTheDocument();
  });

  it('renders FilterBar dropdown filters', () => {
    renderPage();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Severity')).toBeInTheDocument();
    expect(screen.getByText('Type')).toBeInTheDocument();
  });

  it('renders quick filter buttons', () => {
    renderPage();
    expect(screen.getByText('All Updates')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Critical' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pending Approval' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approved' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Installing' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Installed' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Failed' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Dependencies' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Vulnerable' })).toBeInTheDocument();
  });

  it('quick filter "Critical" sets severity=critical and status=pending', () => {
    renderPage();
    const criticalBtn = screen.getByRole('button', { name: 'Critical' });
    fireEvent.click(criticalBtn);
    // Filter pills should show the active filters
    expect(screen.getByText('Status:')).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
    expect(screen.getByText('Severity:')).toBeInTheDocument();
    expect(screen.getByText('critical')).toBeInTheDocument();
  });

  it('quick filter "Pending Approval" sets status=pending and clears severity', () => {
    renderPage();
    const pendingBtn = screen.getByRole('button', { name: 'Pending Approval' });
    fireEvent.click(pendingBtn);
    // Should show status pending pill but no severity pill
    expect(screen.getByText('Status:')).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
    expect(screen.queryByText('Severity:')).not.toBeInTheDocument();
  });

  it('quick filter "All Updates" clears all filters', () => {
    renderPage();
    // First set a filter via a quick filter
    fireEvent.click(screen.getByRole('button', { name: 'Critical' }));
    expect(screen.getByText('Status:')).toBeInTheDocument();
    expect(screen.getByText('Severity:')).toBeInTheDocument();
    // Then click "All Updates" to clear
    fireEvent.click(screen.getByText('All Updates'));
    // All pills should be gone
    expect(screen.queryByText('Status:')).not.toBeInTheDocument();
    expect(screen.queryByText('Severity:')).not.toBeInTheDocument();
    expect(screen.queryByText('clear all')).not.toBeInTheDocument();
  });

  it('clear all shows and functions when filters active', () => {
    renderPage();
    // Set a filter via quick filter
    fireEvent.click(screen.getByRole('button', { name: 'Critical' }));
    // "clear all" should appear (use getByText — throws if missing)
    const clearAll = screen.getByText('clear all');
    fireEvent.click(clearAll);
    // All filters should be cleared — no pills, no "clear all"
    expect(screen.queryByText('Status:')).not.toBeInTheDocument();
    expect(screen.queryByText('Severity:')).not.toBeInTheDocument();
    expect(screen.queryByText('clear all')).not.toBeInTheDocument();
  });

  it('renders Command History button', () => {
    renderPage();
    expect(screen.getByText('Command History')).toBeInTheDocument();
  });

  it('renders search input that accepts text', () => {
    renderPage();
    const input = screen.getByPlaceholderText('Search updates by package name...');
    fireEvent.change(input, { target: { value: 'kernel' } });
    expect(input).toHaveValue('kernel');
  });

  it('renders filter dropdowns', () => {
    renderPage();
    // The page should render combobox elements from the FilterDropdowns
    const combos = screen.getAllByRole('combobox');
    expect(combos.length).toBeGreaterThanOrEqual(3); // Status + Severity + Type
  });
});
