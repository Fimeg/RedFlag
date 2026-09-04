import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi } from 'vitest';

// Mock all API hooks
vi.mock('../hooks/useCommands', () => ({
  useActiveCommands: vi.fn(() => ({
    data: { commands: [] },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  })),
  useRetryCommand: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useCancelCommand: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useClearFailedCommands: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock('../hooks/useAgents', () => ({
  useAgents: vi.fn(() => ({
    data: { agents: [] },
    isPending: false,
  })),
}));

vi.mock('../hooks/useUpdates', () => ({
  useUpdates: vi.fn(() => ({
    data: { updates: [] },
    isPending: false,
  })),
}));

vi.mock('../lib/api', () => ({
  logApi: {
    getActiveOperations: vi.fn(() => Promise.resolve({ operations: [] })),
  },
}));

import LiveOperations from './LiveOperations';

function renderPage(initialEntries = ['/live-operations']) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <LiveOperations />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('LiveOperations page filter integration', () => {
  it('renders the page header', () => {
    renderPage();
    expect(screen.getByText('Staging')).toBeInTheDocument();
  });

  it('renders FilterBar with search input', () => {
    renderPage();
    expect(screen.getByPlaceholderText('Search by package name or agent...')).toBeInTheDocument();
  });

  it('renders Status filter dropdown', () => {
    renderPage();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All Status')).toBeInTheDocument();
  });

  it('changing Status filter updates the dropdown', () => {
    renderPage();
    const statusSelect = screen.getByDisplayValue('All Status');
    fireEvent.change(statusSelect, { target: { value: 'running' } });
    expect(statusSelect).toHaveValue('running');
  });

  it('renders auto-refresh toggle', () => {
    renderPage();
    expect(screen.getByText('Auto Refresh')).toBeInTheDocument();
  });

  it('renders Refresh Now button', () => {
    renderPage();
    expect(screen.getByText('Refresh Now')).toBeInTheDocument();
  });

  it('shows empty state when no operations', () => {
    renderPage();
    expect(screen.getByText('No active operations')).toBeInTheDocument();
  });

  it('search input accepts text', () => {
    renderPage();
    const input = screen.getByPlaceholderText('Search by package name or agent...');
    fireEvent.change(input, { target: { value: 'nginx' } });
    expect(input).toHaveValue('nginx');
  });
});
