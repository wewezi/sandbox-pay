import { WinstonModule } from 'nest-winston';
import * as winston from 'winston';

/**
 * Structured JSON logging for production, human-readable console for development.
 * Log level is controlled via the LOG_LEVEL env var (default: 'info').
 *
 * In production, every log line is a single JSON object with:
 *   { timestamp, level, message, context, ...extra }
 *
 * Sensitive fields are NOT redacted here — that happens in the global
 * exception filter (AllExceptionsFilter) before the error reaches the logger.
 */
export function createWinstonLogger() {
  const isProd = process.env.NODE_ENV === 'production';
  const level = process.env.LOG_LEVEL || (isProd ? 'info' : 'debug');

  const formats = isProd
    ? [winston.format.timestamp(), winston.format.json()]
    : [
        winston.format.timestamp({ format: 'HH:mm:ss' }),
        winston.format.colorize(),
        winston.format.printf(({ timestamp, level, message, context }) => {
          const ctx = context ? `[${context}]` : '';
          return `${timestamp} ${level} ${ctx} ${message}`;
        }),
      ];

  return WinstonModule.createLogger({
    level,
    transports: [new winston.transports.Console({ format: winston.format.combine(...formats) })],
  });
}
