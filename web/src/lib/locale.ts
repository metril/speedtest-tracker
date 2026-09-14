/** regionFromLocale extracts the ISO 3166-1 alpha-2 region subtag from a
 * BCP 47 locale tag, e.g. "en-US" -> "US", "en-GB" -> "GB",
 * "zh-Hans-CN" -> "CN" (skipping the 4-letter script subtag), "fr" ->
 * undefined (no region present). Pure and side-effect free -- callers
 * pass it `navigator.language` themselves, which keeps this testable
 * without stubbing globals. */
export function regionFromLocale(locale: string): string | undefined {
  if (!locale) return undefined;
  const parts = locale.split(/[-_]/).slice(1);
  const region = parts.find((p) => /^[A-Za-z]{2}$/.test(p));
  return region ? region.toUpperCase() : undefined;
}
