import { describe, expect, it, vi, afterEach } from 'vitest';
import { newId } from './id';

describe('newId', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('uses crypto.randomUUID when present', () => {
    const spy = vi.spyOn(crypto, 'randomUUID').mockReturnValue('abcd1234-0000-0000-0000-000000000000');
    const id = newId(8);
    expect(spy).toHaveBeenCalled();
    expect(id).toBe('abcd1234');
  });

  it('falls back to getRandomValues when randomUUID is unavailable', () => {
    const original = crypto.randomUUID;
    // @ts-expect-error -- simulate a non-secure context missing randomUUID
    delete crypto.randomUUID;
    try {
      const id = newId(8);
      expect(id).toMatch(/^[0-9a-f]{8}$/);
    } finally {
      crypto.randomUUID = original;
    }
  });

  it('returns 8 lowercase hex chars', () => {
    const id = newId(8);
    expect(id).toMatch(/^[0-9a-f]{8}$/);
  });

  it('generates unique ids', () => {
    const ids = new Set(Array.from({ length: 50 }, () => newId(8)));
    expect(ids.size).toBe(50);
  });
});
