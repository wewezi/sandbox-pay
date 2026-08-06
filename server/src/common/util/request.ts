import type { Request } from 'express';

/**
 * Best-effort client IP. Prefers `X-Forwarded-For` (when behind a proxy you
 * trust) and falls back to the socket address.
 */
export function clientIp(req: Request | undefined): string | undefined {
  const fwd = req?.headers?.['x-forwarded-for'];
  const fwdStr = Array.isArray(fwd) ? fwd[0] : fwd;
  return (
    fwdStr?.toString().split(',')[0].trim() ||
    req?.ip ||
    req?.socket?.remoteAddress
  );
}
