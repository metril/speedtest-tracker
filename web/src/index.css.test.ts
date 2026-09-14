import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const css = readFileSync(resolve(__dirname, 'index.css'), 'utf8');

/** The app's own @theme block already defines --color-accent (a TEXT
 * color, #0369a1/#38bdf8) and --color-muted (a TEXT color,
 * #475569/#94a3b8). shadcn's aliases separately define --accent/--muted
 * as a light-gray SURFACE color. The @theme inline block re-exposes
 * shadcn's aliases as Tailwind utilities -- if it redefines
 * --color-accent, --color-accent-foreground or --color-muted there, it
 * silently overrides the app's own tokens of the same name, and every
 * text-accent/text-muted/bg-accent in the app renders as
 * near-invisible gray-on-gray instead of the intended color. */
describe('index.css @theme inline block', () => {
  const inlineBlock = css.slice(css.indexOf('@theme inline'));

  it('does not redefine --color-accent', () => {
    expect(inlineBlock).not.toMatch(/--color-accent:/);
  });

  it('does not redefine --color-accent-foreground', () => {
    expect(inlineBlock).not.toMatch(/--color-accent-foreground:/);
  });

  it('does not redefine --color-muted', () => {
    expect(inlineBlock).not.toMatch(/--color-muted:/);
  });

  it('still exposes --color-muted-foreground (no collision with this one)', () => {
    expect(inlineBlock).toMatch(/--color-muted-foreground:/);
  });
});
