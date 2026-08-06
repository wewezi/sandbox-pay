import { Body, Controller, Get, Post, Req, Res, UseGuards } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { Throttle } from '@nestjs/throttler';
import { ApiOperation, ApiTags } from '@nestjs/swagger';
import type { Request, Response } from 'express';
import { AuthService } from './auth.service';
import { LoginDto } from './dto/login.dto';
import { SANDBOX_PAY_ACCESS_COOKIE } from './jwt.strategy';

const ACCESS_COOKIE_MAX_AGE_MS = 24 * 60 * 60 * 1000; // 24h — matches default JWT_EXPIRES_IN

@ApiTags('Auth')
@Controller('api/auth')
export class AuthController {
  constructor(private readonly authService: AuthService) {}

  @Post('login')
  @Throttle({ default: { limit: 5, ttl: 60_000 } }) // 5 attempts / minute — brute-force protection
  @ApiOperation({ summary: '管理员账号密码登录' })
  async login(
    @Body() body: LoginDto,
    @Res({ passthrough: true }) res: Response,
  ) {
    const user = await this.authService.validateUser(body.username, body.password);
    const result = this.authService.issueLocalToken(user);

    // Set the session as an HttpOnly cookie so the browser sends it automatically.
    // The token is also returned in the body for clients that prefer Bearer auth.
    res.cookie(SANDBOX_PAY_ACCESS_COOKIE, result.token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      path: '/',
      maxAge: ACCESS_COOKIE_MAX_AGE_MS,
    });

    return {
      token: result.token,
      tokenType: result.tokenType,
      username: result.username,
      role: result.role,
    };
  }

  @Get('session')
  @UseGuards(AuthGuard('jwt'))
  session(@Req() req: Request) {
    return req.user;
  }

  @Post('logout')
  logout(@Res({ passthrough: true }) res: Response) {
    res.clearCookie(SANDBOX_PAY_ACCESS_COOKIE, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      path: '/',
    });
    return { message: 'Logged out' };
  }
}
