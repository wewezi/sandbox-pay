/**
 * Shared CORS origin validator.
 * Allows localhost (any port) and *.we29.cn subdomains.
 */
export function isCorsAllowed(origin: string | undefined): boolean {
  // Allow requests with no origin (server-to-server, curl, mobile apps)
  if (!origin) return true;

  try {
    const { hostname } = new URL(origin);
    return (
      hostname === 'localhost' ||
      hostname === '127.0.0.1' ||
      hostname.endsWith('.we29.cn') ||
      hostname === 'we29.cn'
    );
  } catch (_) {
    return false;
  }
}

/**
 * Dynamic origin callback for Express CORS middleware.
 */
export function corsOriginDelegate(
  origin: string | undefined,
  callback: (err: Error | null, allow?: boolean) => void,
): void {
  if (isCorsAllowed(origin)) {
    callback(null, true);
  } else {
    callback(new Error(`CORS blocked: ${origin}`));
  }
}
