import {
  ExceptionFilter,
  Catch,
  ArgumentsHost,
  HttpException,
  HttpStatus,
  Logger,
} from '@nestjs/common';
import { Response } from 'express';
import { redact } from '../logging/redact';
import type { AuthenticatedRequest } from '../types/express';

@Catch()
export class AllExceptionsFilter implements ExceptionFilter {
  private readonly logger = new Logger(AllExceptionsFilter.name);

  catch(exception: unknown, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse<Response>();
    const request = ctx.getRequest<AuthenticatedRequest>();
    const status =
      exception instanceof HttpException
        ? exception.getStatus()
        : HttpStatus.INTERNAL_SERVER_ERROR;

    const message =
      exception instanceof HttpException
        ? exception.getResponse()
        : 'Internal server error';

    const safeMessage = redact(message);
    this.logger.error(
      `Http Status: ${status} Error Message: ${JSON.stringify(safeMessage)}`,
      exception instanceof Error ? exception.stack : '',
    );

    const msgText = typeof message === 'object' && message !== null
      ? (message as Record<string, unknown>).message ?? JSON.stringify(message)
      : message;
    const finalMsg = Array.isArray(msgText) ? msgText[0] : String(msgText);

    response.status(status).json({
      statusCode: status,
      code: status,
      timestamp: new Date().toISOString(),
      path: request.url,
      message: finalMsg,
      msg: finalMsg,
      correlationId: request.correlationId,
    });
  }
}
