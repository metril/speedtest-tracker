import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useUnsavedGuard } from './useUnsavedGuard';

describe('useUnsavedGuard', () => {
  it('runs the action immediately when nothing is dirty', () => {
    const discard = vi.fn();
    const action = vi.fn();
    const { result } = renderHook(() => useUnsavedGuard(false, false, discard));

    act(() => result.current.requestNavigate(action));

    expect(action).toHaveBeenCalledTimes(1);
    expect(result.current.confirmOpen).toBe(false);
  });

  it('holds the action behind a confirm dialog when dirty, and discards + runs it on confirm', () => {
    const discard = vi.fn();
    const action = vi.fn();
    const { result } = renderHook(() => useUnsavedGuard(true, true, discard));

    act(() => result.current.requestNavigate(action));
    expect(action).not.toHaveBeenCalled();
    expect(result.current.confirmOpen).toBe(true);

    act(() => result.current.confirmDiscard());
    expect(discard).toHaveBeenCalledTimes(1);
    expect(action).toHaveBeenCalledTimes(1);
    expect(result.current.confirmOpen).toBe(false);
  });

  it('cancel hides the dialog without discarding or running the action', () => {
    const discard = vi.fn();
    const action = vi.fn();
    const { result } = renderHook(() => useUnsavedGuard(true, true, discard));

    act(() => result.current.requestNavigate(action));
    act(() => result.current.cancel());

    expect(discard).not.toHaveBeenCalled();
    expect(action).not.toHaveBeenCalled();
    expect(result.current.confirmOpen).toBe(false);
  });

  it('prevents beforeunload only while any section is dirty', () => {
    const { result, rerender } = renderHook(
      ({ anyDirty }) => useUnsavedGuard(anyDirty, false, vi.fn()),
      { initialProps: { anyDirty: false } },
    );
    void result;

    const clean = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);

    rerender({ anyDirty: true });
    const dirty = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(dirty);
    expect(dirty.defaultPrevented).toBe(true);
  });
});
