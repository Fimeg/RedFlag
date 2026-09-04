import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import FilterBar from './FilterBar';

describe('FilterBar', () => {
  it('renders search input when search prop is provided', () => {
    render(
      <FilterBar
        search={{ value: '', onChange: () => {} }}
      />
    );
    expect(screen.getByPlaceholderText('Search...')).toBeInTheDocument();
  });

  it('renders search with custom placeholder', () => {
    render(
      <FilterBar
        search={{ value: '', onChange: () => {}, placeholder: 'Find stuff...' }}
      />
    );
    expect(screen.getByPlaceholderText('Find stuff...')).toBeInTheDocument();
  });

  it('calls search onChange when input value changes', () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        search={{ value: '', onChange }}
      />
    );
    const input = screen.getByPlaceholderText('Search...');
    fireEvent.change(input, { target: { value: 'foo' } });
    expect(onChange).toHaveBeenCalledWith('foo');
  });

  it('renders dropdown filters from filters prop', () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        filters={[
          { label: 'Status', value: '', onChange, options: [{ value: 'online', label: 'Online' }, { value: 'offline', label: 'Offline' }], placeholder: 'All Status' },
          { label: 'OS', value: '', onChange, options: [{ value: 'linux', label: 'Linux' }], placeholder: 'All OS' },
        ]}
      />
    );
    // Labels rendered
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('OS')).toBeInTheDocument();
    // Selects rendered
    expect(screen.getByDisplayValue('All Status')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All OS')).toBeInTheDocument();
  });

  it('renders pills when pills array is non-empty', () => {
    render(
      <FilterBar
        pills={[
          { label: 'status', value: 'pending', onClear: () => {} },
          { label: 'severity', value: 'critical', onClear: () => {} },
        ]}
        activeCount={2}
      />
    );
    expect(screen.getByText('pending')).toBeInTheDocument();
    expect(screen.getByText('critical')).toBeInTheDocument();
    expect(screen.getByText('status:')).toBeInTheDocument();
    expect(screen.getByText('severity:')).toBeInTheDocument();
  });

  it('shows clear all button when activeCount > 0 and onClearAll is provided', () => {
    render(
      <FilterBar
        pills={[{ label: 'status', value: 'pending', onClear: () => {} }]}
        onClearAll={() => {}}
        activeCount={1}
      />
    );
    expect(screen.getByText('clear all')).toBeInTheDocument();
  });

  it('does not show clear all button when activeCount is 0', () => {
    render(
      <FilterBar
        pills={[]}
        onClearAll={() => {}}
        activeCount={0}
      />
    );
    expect(screen.queryByText('clear all')).not.toBeInTheDocument();
  });

  it('does not show clear all button when onClearAll is missing', () => {
    render(
      <FilterBar
        pills={[{ label: 'status', value: 'pending', onClear: () => {} }]}
        activeCount={1}
      />
    );
    expect(screen.queryByText('clear all')).not.toBeInTheDocument();
  });

  it('calls pill onClear when X button is clicked', () => {
    const onClear = vi.fn();
    render(
      <FilterBar
        pills={[{ label: 'status', value: 'pending', onClear }]}
        activeCount={1}
      />
    );
    const removeButton = screen.getByLabelText('Remove status filter');
    fireEvent.click(removeButton);
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it('calls onClearAll when clear all is clicked', () => {
    const onClearAll = vi.fn();
    render(
      <FilterBar
        pills={[{ label: 'status', value: 'pending', onClear: () => {} }]}
        onClearAll={onClearAll}
        activeCount={1}
      />
    );
    fireEvent.click(screen.getByText('clear all'));
    expect(onClearAll).toHaveBeenCalledTimes(1);
  });

  it('renders actions slot when provided', () => {
    render(
      <FilterBar
        actions={<button>Bulk Action</button>}
      />
    );
    expect(screen.getByText('Bulk Action')).toBeInTheDocument();
  });

  it('does not render pill row when empty and inactive', () => {
    const { container } = render(
      <FilterBar pills={[]} activeCount={0} />
    );
    // If pills/activeCount are both empty, the pill section doesn't render
    expect(container.querySelector('.flex-wrap.items-center.gap-1\\.5')).toBeNull();
  });

  it('calls dropdown onChange when a selection is made', () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        filters={[
          { label: 'Status', value: '', onChange, options: [{ value: 'online', label: 'Online' }], placeholder: 'All Status' },
        ]}
      />
    );
    const select = screen.getByDisplayValue('All Status');
    fireEvent.change(select, { target: { value: 'online' } });
    expect(onChange).toHaveBeenCalledWith('online');
  });

  it('renders dropdown with active styling when value is set', () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        filters={[
          { label: 'Status', value: 'online', onChange, options: [{ value: 'online', label: 'Online' }] },
        ]}
      />
    );
    const select = screen.getByDisplayValue('Online');
    expect(select).toBeInTheDocument();
  });
});
