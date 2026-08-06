import { SignatureService } from './signature.service';

describe('SignatureService', () => {
  let service: SignatureService;

  beforeEach(() => {
    service = new SignatureService();
  });

  describe('sign', () => {
    it('produces a lowercase hex HMAC-SHA256', () => {
      const sig = service.sign('hello', 'secret');
      expect(sig).toMatch(/^[0-9a-f]{64}$/);
    });

    it('is deterministic', () => {
      const sig1 = service.sign('payload', 'key');
      const sig2 = service.sign('payload', 'key');
      expect(sig1).toBe(sig2);
    });

    it('differs for different payloads', () => {
      const sig1 = service.sign('payload1', 'key');
      const sig2 = service.sign('payload2', 'key');
      expect(sig1).not.toBe(sig2);
    });

    it('differs for different secrets', () => {
      const sig1 = service.sign('payload', 'key1');
      const sig2 = service.sign('payload', 'key2');
      expect(sig1).not.toBe(sig2);
    });
  });

  describe('verify', () => {
    it('returns true for a valid signature', () => {
      const secret = 'test-secret';
      const payload = 'test-payload';
      const signature = service.sign(payload, secret);
      expect(service.verify(payload, secret, signature)).toBe(true);
    });

    it('returns false for an invalid signature', () => {
      expect(service.verify('payload', 'secret', 'invalid-signature')).toBe(false);
    });

    it('returns false for empty signature', () => {
      expect(service.verify('payload', 'secret', '')).toBe(false);
    });

    it('returns false for null/undefined signature', () => {
      expect(service.verify('payload', 'secret', null as any)).toBe(false);
      expect(service.verify('payload', 'secret', undefined as any)).toBe(false);
    });

    it('is case-insensitive (normalizes to lowercase)', () => {
      const secret = 'test-secret';
      const payload = 'test-payload';
      const sig = service.sign(payload, secret);
      expect(service.verify(payload, secret, sig.toUpperCase())).toBe(true);
    });
  });

  describe('buildPayload', () => {
    it('concatenates body, timestamp, and nonce', () => {
      const result = service.buildPayload('{"amount":100}', '1234567890', 'nonce-abc');
      expect(result).toBe('{"amount":100}&timestamp=1234567890&nonce=nonce-abc');
    });
  });

  describe('canonicalizeQuery', () => {
    it('sorts keys lexicographically', () => {
      const result = service.canonicalizeQuery({ z: '1', a: '2', m: '3' });
      expect(result).toBe('a=2&m=3&z=1');
    });

    it('skips null/undefined values', () => {
      const result = service.canonicalizeQuery({ a: '1', b: null, c: undefined, d: '4' });
      expect(result).toBe('a=1&d=4');
    });

    it('joins arrays with comma', () => {
      const result = service.canonicalizeQuery({ tags: ['a', 'b', 'c'] });
      expect(result).toBe('tags=a,b,c');
    });

    it('returns empty string for null/undefined input', () => {
      expect(service.canonicalizeQuery(null)).toBe('');
      expect(service.canonicalizeQuery(undefined)).toBe('');
    });

    it('returns empty string for empty object', () => {
      expect(service.canonicalizeQuery({})).toBe('');
    });
  });
});
