import { expect, it } from 'vitest';
import { regionFromLocale } from './locale';

it('extracts a simple region subtag', () => {
  expect(regionFromLocale('en-US')).toBe('US');
  expect(regionFromLocale('en-GB')).toBe('GB');
});

it('uppercases a lowercase region subtag', () => {
  expect(regionFromLocale('en-us')).toBe('US');
});

it('skips a script subtag to find the region', () => {
  expect(regionFromLocale('zh-Hans-CN')).toBe('CN');
});

it('returns undefined for a language-only tag', () => {
  expect(regionFromLocale('fr')).toBeUndefined();
});

it('returns undefined for an empty string', () => {
  expect(regionFromLocale('')).toBeUndefined();
});

it('handles underscore-separated tags', () => {
  expect(regionFromLocale('en_US')).toBe('US');
});
