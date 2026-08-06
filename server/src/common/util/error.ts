/**
 * Narrow an unknown caught value to a human-readable message string.
 * Use in `catch (err: unknown)` blocks to avoid leaking `any` while keeping
 * call sites short.
 */
export function errMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  try {
    return JSON.stringify(err);
  } catch (_) {
    return String(err);
  }
}

export function errStack(err: unknown): string | undefined {
  return err instanceof Error ? err.stack : undefined;
}
