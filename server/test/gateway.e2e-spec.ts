import { INestApplication } from '@nestjs/common';
import request from 'supertest';
import { App } from 'supertest/types';
import { createTestApp } from './helpers/test-app.helper';
import { SignatureService } from '../src/common/services/signature.service';

describe('Gateway (e2e)', () => {
  let app: INestApplication<App>;
  let signatureService: SignatureService;

  const APP_KEY = 'sp_test_key';
  const APP_SECRET = 'test-secret'; // matches mock merchant

  beforeAll(async () => {
    const testApp = await createTestApp();
    app = testApp.app;
    signatureService = app.get(SignatureService);
  });

  afterAll(async () => {
    await app.close();
  });

  /** Build signed headers for a gateway request. */
  function signRequest(body: Record<string, unknown> = {}) {
    const timestamp = Date.now().toString();
    const nonce = `nonce-${Math.random().toString(36).slice(2)}`;
    const bodyStr = JSON.stringify(body);
    const payload = signatureService.buildPayload(bodyStr, timestamp, nonce);
    const signature = signatureService.sign(payload, APP_SECRET);

    return {
      'X-Sandbox-Pay-AppKey': APP_KEY,
      'X-Sandbox-Pay-Timestamp': timestamp,
      'X-Sandbox-Pay-Nonce': nonce,
      'X-Sandbox-Pay-Signature': signature,
      'Content-Type': 'application/json',
    };
  }

  describe('auth guard', () => {
    it('rejects requests without signature headers', () => {
      return request(app.getHttpServer())
        .post('/api/gateway/pay')
        .send({ amount: 100, productName: 'Test', payMethod: 'native' })
        .expect(401);
    });

    it('rejects requests with invalid signature', () => {
      return request(app.getHttpServer())
        .post('/api/gateway/pay')
        .set({
          ...signRequest({ amount: 100 }),
          'X-Sandbox-Pay-Signature': 'invalid-signature-value',
        })
        .send({ amount: 100, productName: 'Test', payMethod: 'native' })
        .expect(401);
    });

    it('rejects requests with expired timestamp', () => {
      const timestamp = (Date.now() - 10 * 60 * 1000).toString(); // 10 minutes ago
      const nonce = 'old-nonce';
      const body = { amount: 100, productName: 'Test', payMethod: 'native' };
      const payload = signatureService.buildPayload(JSON.stringify(body), timestamp, nonce);
      const signature = signatureService.sign(payload, APP_SECRET);

      return request(app.getHttpServer())
        .post('/api/gateway/pay')
        .set({
          'X-Sandbox-Pay-AppKey': APP_KEY,
          'X-Sandbox-Pay-Timestamp': timestamp,
          'X-Sandbox-Pay-Nonce': nonce,
          'X-Sandbox-Pay-Signature': signature,
          'Content-Type': 'application/json',
        })
        .send(body)
        .expect(401);
    });
  });

  describe('POST /api/gateway/pay', () => {
    it('rejects invalid payMethod', () => {
      return request(app.getHttpServer())
        .post('/api/gateway/pay')
        .set(signRequest({ amount: 100, productName: 'Test', payMethod: 'invalid' }))
        .send({ amount: 100, productName: 'Test', payMethod: 'invalid' })
        .expect(400);
    });

    it('rejects missing required fields', () => {
      return request(app.getHttpServer())
        .post('/api/gateway/pay')
        .set(signRequest({}))
        .send({})
        .expect(400);
    });

    it('rejects amount below minimum', () => {
      return request(app.getHttpServer())
        .post('/api/gateway/pay')
        .set(signRequest({ amount: 0, productName: 'Test', payMethod: 'native' }))
        .send({ amount: 0, productName: 'Test', payMethod: 'native' })
        .expect(400);
    });
  });

  describe('GET /api/gateway/query', () => {
    it('rejects query without orderNo', () => {
      const timestamp = Date.now().toString();
      const nonce = `nonce-${Math.random().toString(36).slice(2)}`;
      const canonical = ''; // empty query
      const payload = signatureService.buildPayload(canonical, timestamp, nonce);
      const signature = signatureService.sign(payload, APP_SECRET);

      return request(app.getHttpServer())
        .get('/api/gateway/query')
        .set({
          'X-Sandbox-Pay-AppKey': APP_KEY,
          'X-Sandbox-Pay-Timestamp': timestamp,
          'X-Sandbox-Pay-Nonce': nonce,
          'X-Sandbox-Pay-Signature': signature,
        })
        .expect(404); // order not found
    });
  });
});
