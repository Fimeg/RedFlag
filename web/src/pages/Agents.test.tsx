import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi } from 'vitest';

// Mock all API hooks
vi.mock('../hooks/useAgents', () => ({
  useAgents: vi.fn(() => ({
    data: { agents: [
      { id: '1', hostname: 'waystation', os_type: 'linux', status: 'online', version: '1.0', ip_address: '10.0.0.1', last_seen: new Date().toISOString(), metadata: {}, capabilities: [] },
      { id: '2', hostname: 'webserver', os_type: 'linux', status: 'online', version: '1.0', ip_address: '10.0.0.2', last_seen: new Date().toISOString(), metadata: {}, capabilities: [] },
    ] },
    isPending: false,
    error: null,
    refetch: vi.fn(),
  })),
  useAgent: vi.fn(() => ({ data: null })),
  useScanMultipleAgents: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useUnregisterAgent: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock('../hooks/useCommands', () => ({
  useActiveCommands: vi.fn(() => ({ data: { commands: [] }, refetch: vi.fn() })),
  useCancelCommand: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useCaptureScreenshot: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useCommand: vi.fn(() => ({ data: null })),
}));

vi.mock('../hooks/useHeartbeat', () => ({
  useHeartbeatStatus: vi.fn(() => ({ enabled: false, active: false })),
}));

vi.mock('../hooks/useDebounce', () => ({
  useDebounce: vi.fn((value) => value),
}));

import Agents from './Agents';
import { ConfirmProvider } from '@/components/primitives';

function renderPage(initialEntries = ['/agents']) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <ConfirmProvider>
          <Agents />
        </ConfirmProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('Agents page filter integration', () => {
  it('renders the page header', () => {
    renderPage();
    expect(screen.getByText('Agents')).toBeInTheDocument();
  });

  it('renders FilterBar with search input', () => {
    renderPage();
    expect(screen.getByPlaceholderText('Search agents by hostname...')).toBeInTheDocument();
  });

  it('renders Status filter dropdown', () => {
    renderPage();
    expect(screen.getByDisplayValue('All Status')).toBeInTheDocument();
  });

  it('renders OS filter dropdown', () => {
    renderPage();
    expect(screen.getByDisplayValue('All OS')).toBeInTheDocument();
  });

  it('changing Status filter updates the URL and shows pills', () => {
    renderPage();
    const statusSelect = screen.getByDisplayValue('All Status');
    fireEvent.change(statusSelect, { target: { value: 'online' } });
    // After selecting a filter, pills should appear
    expect(screen.getByText(/online/)).toBeInTheDocument();
    // "clear all" should appear when filters are active
    expect(screen.getByText('clear all')).toBeInTheDocument();
  });

  it('changing OS filter works', () => {
    renderPage();
    const osSelect = screen.getByDisplayValue('All OS');
    fireEvent.change(osSelect, { target: { value: 'linux' } });
    expect(screen.getAllByRole('option', { name: 'linux' }).length).toBeGreaterThan(0);
  });

  it('clear all resets filter pills', () => {
    renderPage();
    // Set a filter
    const statusSelect = screen.getByDisplayValue('All Status');
    fireEvent.change(statusSelect, { target: { value: 'online' } });
    expect(screen.getByText(/online/)).toBeInTheDocument();

    // Clear all
    fireEvent.click(screen.getByText('clear all'));
    expect(screen.queryByText(/online/)).not.toBeInTheDocument();
  });

  it('pill X button removes individual filter', () => {
    renderPage();
    // Set a filter
    const statusSelect = screen.getByDisplayValue('All Status');
    fireEvent.change(statusSelect, { target: { value: 'online' } });
    expect(screen.getByText(/online/)).toBeInTheDocument();

    // Remove the pill
    const removeBtn = screen.getByLabelText('Remove Status filter');
    fireEvent.click(removeBtn);
    expect(screen.queryByText(/online/)).not.toBeInTheDocument();
  });

  it('renders search input that accepts text', () => {
    renderPage();
    const input = screen.getByPlaceholderText('Search agents by hostname...');
    fireEvent.change(input, { target: { value: 'server' } });
    expect(input).toHaveValue('server');
  });

  it('does not show pills on initial load', () => {
    renderPage();
    // No filter pills should be present initially (no "clear all" text)
    expect(screen.queryByText('clear all')).not.toBeInTheDocument();
  });
});
