export type ByteUnit = 'B' | 'KB' | 'MB' | 'GB';

export const BYTE_UNITS: readonly ByteUnit[] = ['B', 'KB', 'MB', 'GB'];

const UNIT_FACTORS: Record<ByteUnit, number> = {
  B: 1,
  KB: 1e3,
  MB: 1e6,
  GB: 1e9,
};

/** toBytes converts a value in the given decimal (1000-based) unit to bytes. */
export function toBytes(value: number, unit: ByteUnit): number {
  return value * UNIT_FACTORS[unit];
}

/** splitBytes picks the largest unit that divides `bytes` evenly, falling
 * back to B for anything that doesn't divide evenly (or isn't a positive
 * finite number). */
export function splitBytes(bytes: number): { value: number; unit: ByteUnit } {
  if (!Number.isFinite(bytes) || bytes === 0) return { value: bytes, unit: 'B' };
  for (let i = BYTE_UNITS.length - 1; i > 0; i -= 1) {
    const unit = BYTE_UNITS[i];
    const factor = UNIT_FACTORS[unit];
    if (bytes % factor === 0) return { value: bytes / factor, unit };
  }
  return { value: bytes, unit: 'B' };
}

/** formatBytes renders a byte count as "<value> <unit>", e.g. "100 KB". */
export function formatBytes(bytes: number): string {
  const { value, unit } = splitBytes(bytes);
  return `${value} ${unit}`;
}
