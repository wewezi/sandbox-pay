import { ExtractJwt, Strategy } from 'passport-jwt';
import { PassportStrategy } from '@nestjs/passport';
import { Injectable } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import type { JwtPayload, JwtUser } from '../common/types/express';

export const SANDBOX_PAY_ACCESS_COOKIE = 'sandbox_pay_access_token';

@Injectable()
export class JwtStrategy extends PassportStrategy(Strategy) {
  constructor(private configService: ConfigService) {
    super({
      jwtFromRequest: ExtractJwt.fromExtractors([
        ExtractJwt.fromAuthHeaderAsBearerToken(),
        (req) => {
          const value = (req?.cookies as Record<string, unknown> | undefined)?.[
            SANDBOX_PAY_ACCESS_COOKIE
          ];
          return typeof value === 'string' ? value : null;
        },
      ]),
      ignoreExpiration: false,
      secretOrKey: configService.get<string>('JWT_SECRET')!,
    });
  }

  async validate(payload: JwtPayload): Promise<JwtUser> {
    return {
      userId: payload.sub,
      username: payload.username,
      role: payload.role,
    };
  }
}
