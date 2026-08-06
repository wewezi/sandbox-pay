/**
 * Money helpers.
 *
 * Invariant: every amount carried in JS/TS code is an integer number of CNY 分
 * (or the equivalent minor unit for the order's currency). The database column
 * type stays `decimal(p, 2)` for human readability; the `moneyColumnTransformer`
 * below bridges the two — yuan-string in the DB row, cents-integer in memory.
 *
 * Rules:
 *   - Never do floating-point arithmetic on amounts. Always work in cents.
 *   - When calling external gateways (Alipay/PayPal), convert to a 2-decimal
 *     string at the boundary using `toYuanString`.
 *   - When receiving from a gateway, convert to cents via `yuanStringToCents`
 *     at the boundary.
 */

/** Convert a yuan-denominated string/number (e.g. "10.00") to integer cents (1000). */
export function yuanStringToCents(value: string | number | null | undefined): number {
  if (value == null || value === '') return 0;
  return Math.round(Number(value) * 100);
}

/** Convert integer cents to a yuan number (e.g. 1000 → 10). */
export function toYuan(cents: number | null | undefined): number {
  if (cents == null) return 0;
  return Math.round(Number(cents)) / 100;
}

/** Convert integer cents to a fixed-2 string (suitable for gateway APIs). */
export function toYuanString(cents: number | null | undefined): string {
  return toYuan(cents).toFixed(2);
}

/**
 * TypeORM column transformer mapping `decimal(p,2)` yuan strings ↔ integer cents.
 * Apply to amount-like columns so all in-memory code can stay in cents.
 */
export const moneyColumnTransformer = {
  to: (cents: number | null | undefined): string => {
    if (cents == null) return '0.00';
    return (Math.round(Number(cents)) / 100).toFixed(2);
  },
  from: (value: string | number | null | undefined): number | null => {
    if (value == null) return null;
    return Math.round(Number(value) * 100);
  },
};
