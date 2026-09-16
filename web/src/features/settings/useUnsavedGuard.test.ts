import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { NavigationGuardProvider, useNavigationGuard } from './NavigationGuardContext';
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

  it('guards external navigation (sidebar) by anyDirty and discards all sections on confirm', () => {
    const discardActive = vi.fn();
    const discardAll = vi.fn();
    const action = vi.fn();
    const { result } = renderHook(() => ({
      guardHook: useUnsavedGuard(true, false, discardActive, discardAll),
      navGuard: useNavigationGuard(),
    }), { wrapper: NavigationGuardProvider });

    // navDirty is false (active tab is clean) but anyDirty is true (some
    // other section is dirty): the tab-switch path lets it through...
    const tabAction = vi.fn();
    act(() => result.current.guardHook.requestNavigate(tabAction));
    expect(tabAction).toHaveBeenCalledTimes(1);

    // ...but the registered external guard (what a sidebar link calls
    // through NavigationGuardContext) still blocks on anyDirty.
    act(() => result.current.navGuard.guard(action));
    expect(action).not.toHaveBeenCalled();
    expect(result.current.guardHook.confirmOpen).toBe(true);
    expect(result.current.guardHook.confirmKind).toBe('leave');

    act(() => result.current.guardHook.confirmDiscard());
    expect(discardAll).toHaveBeenCalledTimes(1);
    expect(discardActive).not.toHaveBeenCalled();
    expect(action).toHaveBeenCalledTimes(1);
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
