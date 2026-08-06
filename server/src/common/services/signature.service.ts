import { Injectable } from '@nestjs/common';
import * as crypto from 'crypto';

@Injectable()
export class SignatureService {
  sign(payload: string, secret: string): string {
    return crypto
      .createHmac('sha256', secret)
      .update(payload)
      .digest('hex')
      .toLowerCase();
  }

  /**
   * Constant-time HMAC comparison to prevent timing attacks.
   */
  verify(payload: string, secret: string, signature: string): boolean {
    if (!signature || typeof signature !== 'string') return false;
    const expected = this.sign(payload, secret);
    const a = Buffer.from(expected, 'utf8');
    const b = Buffer.from(signature.toLowerCase(), 'utf8');
    if (a.length !== b.length) return false;
    return crypto.timingSafeEqual(a, b);
  }

  buildPayload(body: string, timestamp: string, nonce: string): string {
    return `${body}&timestamp=${timestamp}&nonce=${nonce}`;
  }

  /**
   * Canonicalise a query object into `k1=v1&k2=v2` form with keys sorted
   * lexicographically. Used for stable signing of GET requests / form bodies.
   * `null` / `undefined` values are skipped; arrays are joined with `,`.
   */
  canonicalizeQuery(query: Record<string, any> | undefined | null): string {
    if (!query) return '';
    const keys = Object.keys(query).filter(k => query[k] !== undefined && query[k] !== null).sort();
    return keys
      .map(k => {
        const v = Array.isArray(query[k]) ? query[k].join(',') : String(query[k]);
        return `${k}=${v}`;
      })
      .join('&');
  }
}
