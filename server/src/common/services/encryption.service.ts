import { Injectable, Logger, OnModuleInit } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as crypto from 'crypto';
import { errMessage } from '../util/error';

/**
 * AES-256-GCM encryption for at-rest sensitive secrets (e.g. merchant appSecret).
 *
 * Encrypted payload format (base64): "enc:v1:" + base64(iv | tag | ciphertext)
 *
 * Backward compatibility: any value not prefixed with "enc:v1:" is treated as plaintext
 * and returned unchanged from `decrypt`, allowing seamless rollout for existing rows.
 */
@Injectable()
export class EncryptionService implements OnModuleInit {
  private readonly logger = new Logger(EncryptionService.name);
  private readonly prefix = 'enc:v1:';
  private key!: Buffer;

  constructor(private readonly configService: ConfigService) {}

  onModuleInit() {
    const raw = this.configService.get<string>('ENCRYPTION_KEY');
    if (!raw) {
      // Fallback: derive from JWT_SECRET so dev mode still works without extra config.
      // PRODUCTION MUST set ENCRYPTION_KEY explicitly (32 random bytes, base64 or hex).
      const jwt = this.configService.get<string>('JWT_SECRET') || 'sandbox-pay-dev-fallback';
      this.key = crypto.createHash('sha256').update(jwt).digest();
      this.logger.warn(
        'ENCRYPTION_KEY not set — derived key from JWT_SECRET. Configure ENCRYPTION_KEY in production.',
      );
      return;
    }
    let buf: Buffer;
    if (/^[0-9a-fA-F]+$/.test(raw) && raw.length === 64) {
      buf = Buffer.from(raw, 'hex');
    } else {
      try {
        buf = Buffer.from(raw, 'base64');
      } catch (_) {
        buf = Buffer.from(raw, 'utf8');
      }
    }
    if (buf.length !== 32) {
      // Normalise to 32 bytes via SHA-256 if not already.
      buf = crypto.createHash('sha256').update(buf).digest();
    }
    this.key = buf;
  }

  encrypt(plaintext: string): string {
    if (!plaintext) return plaintext;
    if (plaintext.startsWith(this.prefix)) return plaintext;
    const iv = crypto.randomBytes(12);
    const cipher = crypto.createCipheriv('aes-256-gcm', this.key, iv);
    const ct = Buffer.concat([cipher.update(plaintext, 'utf8'), cipher.final()]);
    const tag = cipher.getAuthTag();
    return this.prefix + Buffer.concat([iv, tag, ct]).toString('base64');
  }

  decrypt(value: string): string {
    if (!value) return value;
    if (!value.startsWith(this.prefix)) return value; // legacy plaintext
    try {
      const data = Buffer.from(value.slice(this.prefix.length), 'base64');
      const iv = data.subarray(0, 12);
      const tag = data.subarray(12, 28);
      const ct = data.subarray(28);
      const decipher = crypto.createDecipheriv('aes-256-gcm', this.key, iv);
      decipher.setAuthTag(tag);
      return Buffer.concat([decipher.update(ct), decipher.final()]).toString('utf8');
    } catch (err: unknown) {
      this.logger.error(`Decryption failed: ${errMessage(err)}`);
      throw new Error('敏感数据解密失败');
    }
  }
}
