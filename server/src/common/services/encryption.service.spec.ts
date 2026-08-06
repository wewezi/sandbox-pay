import { ConfigService } from '@nestjs/config';
import { EncryptionService } from './encryption.service';

function createService(encryptionKey?: string, jwtSecret?: string): EncryptionService {
  const configService = {
    get: (key: string) => {
      if (key === 'ENCRYPTION_KEY') return encryptionKey;
      if (key === 'JWT_SECRET') return jwtSecret;
      return undefined;
    },
  } as unknown as ConfigService;

  const service = new EncryptionService(configService);
  service.onModuleInit();
  return service;
}

describe('EncryptionService', () => {
  describe('encrypt / decrypt roundtrip', () => {
    it('encrypts and decrypts a plaintext string', () => {
      const service = createService('a'.repeat(64)); // 32-byte hex key
      const plaintext = 'my-secret-app-key-12345';
      const encrypted = service.encrypt(plaintext);

      expect(encrypted).not.toBe(plaintext);
      expect(encrypted).toMatch(/^enc:v1:/);
      expect(service.decrypt(encrypted)).toBe(plaintext);
    });

    it('produces different ciphertext for the same plaintext (random IV)', () => {
      const service = createService('b'.repeat(64));
      const plaintext = 'same-secret';
      const enc1 = service.encrypt(plaintext);
      const enc2 = service.encrypt(plaintext);

      expect(enc1).not.toBe(enc2); // different IVs
      expect(service.decrypt(enc1)).toBe(plaintext);
      expect(service.decrypt(enc2)).toBe(plaintext);
    });

    it('handles empty string', () => {
      const service = createService('c'.repeat(64));
      expect(service.encrypt('')).toBe('');
      expect(service.decrypt('')).toBe('');
    });
  });

  describe('legacy plaintext passthrough', () => {
    it('returns plaintext unchanged if not prefixed with enc:v1:', () => {
      const service = createService('d'.repeat(64));
      const legacy = 'plain-text-secret-from-old-row';
      expect(service.decrypt(legacy)).toBe(legacy);
    });

    it('does not double-encrypt already encrypted values', () => {
      const service = createService('e'.repeat(64));
      const plaintext = 'test-value';
      const encrypted = service.encrypt(plaintext);
      const doubleEncrypted = service.encrypt(encrypted);
      expect(doubleEncrypted).toBe(encrypted); // should be no-op
    });
  });

  describe('key derivation', () => {
    it('derives key from JWT_SECRET when ENCRYPTION_KEY is unset', () => {
      const service = createService(undefined, 'my-jwt-secret');
      const plaintext = 'test';
      const encrypted = service.encrypt(plaintext);
      expect(service.decrypt(encrypted)).toBe(plaintext);
    });

    it('normalizes non-32-byte keys via SHA-256', () => {
      const service = createService('short-key');
      const plaintext = 'test-value';
      const encrypted = service.encrypt(plaintext);
      expect(service.decrypt(encrypted)).toBe(plaintext);
    });

    it('accepts hex key (64 chars)', () => {
      const hexKey = '0123456789abcdef'.repeat(4); // 64 hex chars = 32 bytes
      const service = createService(hexKey);
      const plaintext = 'hex-key-test';
      const encrypted = service.encrypt(plaintext);
      expect(service.decrypt(encrypted)).toBe(plaintext);
    });
  });

  describe('tamper detection', () => {
    it('throws on corrupted ciphertext', () => {
      const service = createService('f'.repeat(64));
      const encrypted = service.encrypt('valid-data');
      // Tamper with the base64 payload
      const corrupted = encrypted.slice(0, -4) + 'XXXX';
      expect(() => service.decrypt(corrupted)).toThrow();
    });
  });
});
