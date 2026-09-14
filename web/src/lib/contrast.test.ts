import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/** parseTokens pulls `--color-<name>: #rrggbb;` declarations out of one
 * braced CSS block (either `@theme { ... }` for light, or `.dark { ... }`
 * for dark), so the test asserts on the values actually shipped in
 * index.css rather than a copy pasted into the test. */
function parseTokens(block: string): Record<string, string> {
  const tokens: Record<string, string> = {};
  const re = /--color-([a-z0-9-]+):\s*(#[0-9a-fA-F]{6})\s*;/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(block))) {
    tokens[m[1]] = m[2];
  }
  return tokens;
}

function extractBlock(css: string, header: string): string {
  const start = css.indexOf(header);
  if (start === -1) throw new Error(`block not found: ${header}`);
  const braceStart = css.indexOf('{', start);
  let depth = 0;
  for (let i = braceStart; i < css.length; i++) {
    if (css[i] === '{') depth++;
    if (css[i] === '}') {
      depth--;
      if (depth === 0) return css.slice(braceStart + 1, i);
    }
  }
  throw new Error(`unbalanced braces for block: ${header}`);
}

function srgbToLinear(c: number): number {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

function relativeLuminance(hex: string): number {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return 0.2126 * srgbToLinear(r) + 0.7152 * srgbToLinear(g) + 0.0722 * srgbToLinear(b);
}

/** contrastRatio implements the WCAG 2.x formula: (L1+0.05)/(L2+0.05)
 * with L1 the lighter of the two relative luminances. */
function contrastRatio(hexA: string, hexB: string): number {
  const la = relativeLuminance(hexA);
  const lb = relativeLuminance(hexB);
  const [l1, l2] = la >= lb ? [la, lb] : [lb, la];
  return (l1 + 0.05) / (l2 + 0.05);
}

// vitest's test root is the `web/` package directory.
const cssPath = join(process.cwd(), 'src/index.css');
const css = readFileSync(cssPath, 'utf8');

const light = parseTokens(extractBlock(css, '@theme'));
const dark = parseTokens(extractBlock(css, '.dark'));

const TEXT_TOKENS = ['fg', 'muted', 'faint'] as const;
const BG_TOKENS = ['app', 'surface', 'raised'] as const;
const MIN_RATIO = 4.5;

describe('theme token contrast (WCAG AA, normal text)', () => {
  for (const [theme, tokens] of [['light', light], ['dark', dark]] as const) {
    for (const text of TEXT_TOKENS) {
      for (const bg of BG_TOKENS) {
        it(`${theme}: ${text} on ${bg} is >= ${MIN_RATIO}:1`, () => {
          const ratio = contrastRatio(tokens[text], tokens[bg]);
          expect(ratio).toBeGreaterThanOrEqual(MIN_RATIO);
        });
      }
    }
  }
});
