/**
 * Startup environment variable validation.
 *
 * Called once in main.ts before the app listens. Logs clear errors and
 * exits with code 1 when required variables are missing or malformed.
 */

const isProd = process.env.NODE_ENV === 'production';

const REQUIRED_VARS = [
  'DB_HOST',
  'DB_USERNAME',
  'DB_DATABASE',
  'JWT_SECRET',
  // In production, ENCRYPTION_KEY must be set explicitly — deriving from
  // JWT_SECRET couples authentication and data-encryption keys, meaning a
  // single compromise exposes both.
  ...(isProd ? ['ENCRYPTION_KEY'] : []),
] as const;

const OPTIONAL_BUT_RECOMMENDED = [
  'ENCRYPTION_KEY',
  'GATEWAY_SECRET',
] as const;

export function validateEnv(): void {
  const missing: string[] = [];

  for (const key of REQUIRED_VARS) {
    if (!process.env[key] || process.env[key]!.trim() === '') {
      missing.push(key);
    }
  }

  if (missing.length > 0) {
    console.error(
      `\n[ENV VALIDATION] Missing required environment variables:\n` +
        missing.map((k) => `  - ${k}`).join('\n') +
        `\n\nPlease set them in .env or as system environment variables.\n`,
    );
    process.exit(1);
  }

  // Warn (but don't exit) for recommended vars
  const warnings: string[] = [];
  for (const key of OPTIONAL_BUT_RECOMMENDED) {
    if (!process.env[key] || process.env[key]!.trim() === '') {
      warnings.push(key);
    }
  }

  if (warnings.length > 0) {
    console.warn(
      `[ENV VALIDATION] Recommended variables not set (will use fallbacks):\n` +
        warnings.map((k) => `  - ${k}`).join('\n'),
    );
  }

  // Type checks
  const dbPort = Number(process.env.DB_PORT);
  if (process.env.DB_PORT && (!Number.isFinite(dbPort) || dbPort < 1 || dbPort > 65535)) {
    console.error(`\n[ENV VALIDATION] DB_PORT must be a valid port number (1-65535), got: ${process.env.DB_PORT}\n`);
    process.exit(1);
  }

  if (process.env.ENABLE_SANDBOX === 'true') {
    console.warn(
      `[ENV VALIDATION] ⚠️  ENABLE_SANDBOX is true — sandbox endpoints are active. Set to false in production.`,
    );
  }

  // Warn about dev-only fallback key
  if (!isProd && !process.env.ENCRYPTION_KEY) {
    console.warn(
      `[ENV VALIDATION] ⚠️  ENCRYPTION_KEY not set — will derive from JWT_SECRET (dev only, production will refuse to start)`,
    );
  }
}
