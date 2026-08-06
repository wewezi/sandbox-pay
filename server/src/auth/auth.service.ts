import { Injectable, UnauthorizedException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { JwtService } from '@nestjs/jwt';
import * as bcrypt from 'bcrypt';
import { User } from '../entities/user.entity';

@Injectable()
export class AuthService {
  constructor(
    private readonly jwtService: JwtService,
    @InjectRepository(User)
    private readonly userRepository: Repository<User>,
  ) {}

  /**
   * Validate a username/password pair against the users table.
   * Throws UnauthorizedException on any mismatch — the message is intentionally
   * generic so it cannot be used to enumerate valid usernames.
   */
  async validateUser(username: string, password: string): Promise<User> {
    const user = await this.userRepository.findOne({ where: { username } });
    // Run a comparison even when the user is missing to keep timing uniform
    // and avoid leaking account existence via response time.
    const hash = user?.password ?? '$2b$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvali';
    const ok = await bcrypt.compare(password, hash);
    if (!user || !ok) {
      throw new UnauthorizedException('用户名或密码错误');
    }
    return user;
  }

  /**
   * Mint a local HS256 session JWT for a user. Reusable primitive for any
   * future login flow (password login, admin bootstrap, etc.).
   */
  issueLocalToken(user: User) {
    const payload = { sub: user.id, username: user.username, role: user.role };
    return {
      token: this.jwtService.sign(payload),
      tokenType: 'Bearer',
      username: user.username,
      role: user.role,
    };
  }
}
