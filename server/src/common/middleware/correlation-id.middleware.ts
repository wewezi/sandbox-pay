import { Injectable, NestMiddleware } from '@nestjs/common';
import { Response, NextFunction } from 'express';
import { v4 as uuidv4 } from 'uuid';
import type { AuthenticatedRequest } from '../types/express';

@Injectable()
export class CorrelationIdMiddleware implements NestMiddleware {
  use(req: AuthenticatedRequest, res: Response, next: NextFunction) {
    const correlationIdHeader = 'X-Correlation-ID';
    req.correlationId = req.header(correlationIdHeader) ?? uuidv4();
    res.setHeader(correlationIdHeader, req.correlationId);
    next();
  }
}
