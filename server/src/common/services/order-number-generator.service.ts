import { Injectable } from '@nestjs/common';
import * as crypto from 'crypto';

@Injectable()
export class OrderNumberGenerator {
  create(prefix: string, randomBytes = 12): string {
    const randomPart = crypto.randomBytes(randomBytes).toString('hex').toUpperCase();
    return `${prefix.toUpperCase()}${randomPart}`;
  }
}
