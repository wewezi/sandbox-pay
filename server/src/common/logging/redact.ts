/**
 * Sensitive field redaction for structured logging.
 *
 * Used by the global exception filter and any structured logger output to
 * prevent secrets / tokens / signatures from leaking to disk or external sinks.
 */

const SENSITIVE_KEYS = new Set(
  [
    'password',
    'pass',
    'pwd',
    'walletpass',
    'token',
    'authorization',
    'cookie',
    'set-cookie',
    'appsecret',
    'app_secret',
    'secret',
    'signature',
    'sign',
    'x-sandbox-pay-signature',
    'jwt',
    'jwt_secret',
    'encryption_key',
    'private_key',
    'privatekey',
    'alipay_private_key',
    'alipay_public_key',
    'paypal_client_secret',
    'gateway_secret',
  ].map(s => s.toLowerCase()),
);

const REDACTED = '***REDACTED***';

export function redact<T>(value: T, depth = 0): T {
  if (depth > 6) return value;
  if (value == null) return value;
  if (Array.isArray(value)) {
    return value.map(v => redact(v, depth + 1)) as unknown as T;
  }
  if (typeof value === 'object') {
    const out: Record<string, any> = {};
    for (const [k, v] of Object.entries(value as Record<string, any>)) {
      if (SENSITIVE_KEYS.has(k.toLowerCase())) {
        out[k] = REDACTED;
      } else {
        out[k] = redact(v, depth + 1);
      }
    }
    return out as unknown as T;
  }
  if (typeof value === 'string' && value.length > 0) {
    // Catch obvious bearer tokens / JWT payloads even when they appear in free-form strings
    return value
      .replace(/(Bearer\s+)[A-Za-z0-9._-]+/gi, `$1${REDACTED}`)
      .replace(/(eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)/g, REDACTED) as unknown as T;
  }
  return value;
}
