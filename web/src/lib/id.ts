/** Generates a short random id, preferring crypto.randomUUID (only available
 * in secure contexts) and falling back to getRandomValues elsewhere so
 * plain-http origins don't crash on id generation. */
export function newId(len = 8): string {
  if (typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID().replace(/-/g, '').slice(0, len);
  }
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return hex.slice(0, len);
}
