import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import { ConfirmProvider, useConfirm } from './ConfirmDialog';

// Helper: a button that fires confirm() with the given options and records the result.
interface TriggerProps {
  options: Parameters<ReturnType<typeof useConfirm>>[0];
  onResult: (v: boolean) => void;
}
const Trigger: React.FC<TriggerProps> = ({ options, onResult }) => {
  const confirm = useConfirm();
  return (
    <button
      onClick={async () => {
        const result = await confirm(options);
        onResult(result);
      }}
    >
      Open
    </button>
  );
};

const wrap = (props: TriggerProps) =>
  render(
    <ConfirmProvider>
      <Trigger {...props} />
    </ConfirmProvider>,
  );

describe('ConfirmDialog', () => {
  it('renders title and body when opened', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Are you sure?', body: 'This cannot be undone.' }, onResult });

    fireEvent.click(screen.getByText('Open'));

    expect(await screen.findByText('Are you sure?')).toBeInTheDocument();
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument();
  });

  it('resolves true when the confirm button is clicked', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Confirm', body: 'Do it?' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    await screen.findByText('Confirm');

    fireEvent.click(screen.getByRole('button', { name: 'OK' }));

    await waitFor(() => expect(onResult).toHaveBeenCalledWith(true));
    expect(screen.queryByText('Do it?')).not.toBeInTheDocument();
  });

  it('resolves false when the cancel button is clicked', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Confirm', body: 'Really?' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    await screen.findByText('Confirm');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
  });

  it('uses custom confirmLabel and cancelLabel', async () => {
    const onResult = vi.fn();
    wrap({
      options: { title: 'Delete Item', body: 'Permanent.', confirmLabel: 'Delete', cancelLabel: 'Keep', danger: true },
      onResult,
    });

    fireEvent.click(screen.getByText('Open'));
    await screen.findByText('Delete Item');

    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Keep' })).toBeInTheDocument();
  });

  it('resolves true on Enter key for non-danger dialog', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Proceed?', body: 'Go ahead.' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    const body = await screen.findByText('Go ahead.');

    fireEvent.keyDown(body.parentElement!, { key: 'Enter' });

    await waitFor(() => expect(onResult).toHaveBeenCalledWith(true));
  });

  it('does not resolve on Enter key for danger dialog', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Danger', body: 'Careful.', danger: true }, onResult });

    fireEvent.click(screen.getByText('Open'));
    const body = await screen.findByText('Careful.');

    fireEvent.keyDown(body.parentElement!, { key: 'Enter' });

    // Dialog should still be open
    expect(screen.getByText('Careful.')).toBeInTheDocument();
    expect(onResult).not.toHaveBeenCalled();
  });

  it('resolves false on Escape key', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Escape test', body: 'Press Escape.' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    await screen.findByText('Escape test');

    fireEvent.keyDown(document, { key: 'Escape' });

    await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
  });

  it('danger variant confirm button has red styling', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Danger', body: 'Destructive.', danger: true, confirmLabel: 'Destroy' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    const btn = await screen.findByRole('button', { name: 'Destroy' });

    expect(btn.className).toMatch(/red/);
  });

  it('non-danger confirm button has blue styling', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Normal', body: 'Safe.', confirmLabel: 'Proceed' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    const btn = await screen.findByRole('button', { name: 'Proceed' });

    expect(btn.className).toMatch(/blue/);
  });

  it('focuses cancel on danger dialogs so Enter cannot destroy', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Danger', body: 'Careful.', danger: true, confirmLabel: 'Destroy' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    await screen.findByRole('button', { name: 'Destroy' });

    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus();
  });

  it('focuses confirm on non-danger dialogs', async () => {
    const onResult = vi.fn();
    wrap({ options: { title: 'Normal', body: 'Safe.', confirmLabel: 'Proceed' }, onResult });

    fireEvent.click(screen.getByText('Open'));
    const btn = await screen.findByRole('button', { name: 'Proceed' });

    expect(btn).toHaveFocus();
  });

  it('throws when useConfirm is used outside ConfirmProvider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const BadComponent: React.FC = () => {
      useConfirm();
      return null;
    };
    expect(() => render(<BadComponent />)).toThrow('useConfirm must be used within a ConfirmProvider');
    spy.mockRestore();
  });
});
