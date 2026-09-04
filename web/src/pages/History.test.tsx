import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi } from 'vitest';

// Mock ChatTimeline since it's an external component that makes API calls
vi.mock('../components/ChatTimeline', () => ({
  default: vi.fn(() => <div data-testid="chat-timeline">Timeline</div>),
}));

vi.mock('../hooks/useDebounce', () => ({
  useDebounce: vi.fn((value) => value),
}));

import HistoryPage from './History';

function renderPage(initialEntries = ['/history']) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <HistoryPage />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('History page filter integration', () => {
  it('renders the page header', () => {
    renderPage();
    expect(screen.getByText('History & Audit Log')).toBeInTheDocument();
  });

  it('renders the SearchInput', () => {
    renderPage();
    expect(screen.getByPlaceholderText('Search events...')).toBeInTheDocument();
  });

  it('renders the ChatTimeline component', () => {
    renderPage();
    expect(screen.getByTestId('chat-timeline')).toBeInTheDocument();
  });

  it('filters panel is hidden by default', () => {
    renderPage();
    // FilterDropdowns should not be visible initially
    expect(screen.queryByText('Type')).not.toBeInTheDocument();
    expect(screen.queryByText('Severity')).not.toBeInTheDocument();
  });

  it('toggles filter panel on button click', () => {
    renderPage();
    const filterBtn = screen.getByText('Filters');
    fireEvent.click(filterBtn);
    // After clicking, filter dropdowns should be visible
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Severity')).toBeInTheDocument();
  });

  it('shows filter count badge when filters are active', () => {
    renderPage();
    // Open the filters panel
    fireEvent.click(screen.getByText('Filters'));
    // Set a type filter
    const typeSelect = screen.getByDisplayValue('All types');
    fireEvent.change(typeSelect, { target: { value: 'command' } });
    // The filter badge should show count 1
    expect(screen.getByText('1')).toBeInTheDocument();
  });

  it('shows "clear all" when filters are active', () => {
    renderPage();
    // Open filters panel
    fireEvent.click(screen.getByText('Filters'));
    // Set a filter
    const typeSelect = screen.getByDisplayValue('All types');
    fireEvent.change(typeSelect, { target: { value: 'command' } });
    // clear all button should appear
    expect(screen.getByText('clear all')).toBeInTheDocument();
  });

  it('clear filters resets all filters', () => {
    renderPage();
    fireEvent.click(screen.getByText('Filters'));
    // Set both filters
    fireEvent.change(screen.getByDisplayValue('All types'), { target: { value: 'command' } });
    fireEvent.change(screen.getByDisplayValue('All severity'), { target: { value: 'error' } });
    expect(screen.getByText('2')).toBeInTheDocument();
    // Clear all
    fireEvent.click(screen.getByText('clear all'));
    // Filter badge count should be gone
    expect(screen.queryByText('2')).not.toBeInTheDocument();
  });

  it('SearchInput accepts text', () => {
    renderPage();
    const input = screen.getByPlaceholderText('Search events...');
    fireEvent.change(input, { target: { value: 'error' } });
    expect(input).toHaveValue('error');
  });

  it('filter button shows active styling when filters are active', () => {
    renderPage();
    // Open panel
    fireEvent.click(screen.getByText('Filters'));
    // Set a filter
    fireEvent.change(screen.getByDisplayValue('All types'), { target: { value: 'command' } });
    // The filter bar should show an active pill reflecting the filter
    expect(screen.getByText('Type:')).toBeInTheDocument();
    expect(screen.getByText('command')).toBeInTheDocument();
  });
});
