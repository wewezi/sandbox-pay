import type { Request } from 'express';
import type { Merchant } from '../../entities/merchant.entity';

/** Payload shape produced by `JwtStrategy.validate()` and attached as `req.user`. */
export interface JwtUser {
  userId: number | string;
  username?: string;
  role?: string;
  sub?: number | string;
}

/** Raw JWT body received by `JwtStrategy.validate(payload)`. */
export interface JwtPayload {
  sub: number | string;
  username?: string;
  role?: string;
  [key: string]: unknown;
}

/**
 * Request decorated by `JwtAuthGuard` (Passport attaches `user`) and/or
 * `MerchantSignatureGuard` (attaches the resolved `merchant`).
 * `CorrelationIdMiddleware` attaches `correlationId`.
 */
export interface AuthenticatedRequest extends Request {
  user?: JwtUser;
  merchant?: Merchant;
  correlationId?: string;
}
