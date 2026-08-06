import { INestApplication } from '@nestjs/common';
import request from 'supertest';
import { createTestApp, generateTestJwt } from './helpers/test-app.helper';
import { JwtService } from '@nestjs/jwt';

/**
 * Payment lifecycle e2e test.
 *
 * Tests: stats → transactions → refund validation → gateway auth → confirm auth.
 * Uses mock repositories from createTestApp helper.
 */
describe('Payment Flow (e2e)', () => {
  let app: INestApplication;
  let repos: any;
  let jwtService: JwtService;
  let adminToken: string;

  beforeAll(async () => {
    const testApp = await createTestApp();
    app = testApp.app;
    repos = testApp.repos;
    jwtService = testApp.jwtService;
    adminToken = generateTestJwt(jwtService);
  });

  afterAll(async () => {
    await app.close();
  });

  describe('Admin endpoints with JWT', () => {
    it('GET /admin/stats returns stats', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/admin/stats')
        .set('Authorization', `Bearer ${adminToken}`);

      expect(res.status).toBe(200);
      expect(res.body.data).toBeDefined();
    });

    it('GET /admin/transactions returns paginated list', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/admin/transactions')
        .set('Authorization', `Bearer ${adminToken}`);

      expect(res.status).toBe(200);
      expect(res.body.data).toBeDefined();
    });

    it('GET /admin/merchant returns merchant info', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/admin/merchant')
        .set('Authorization', `Bearer ${adminToken}`);

      expect(res.status).toBe(200);
      expect(res.body.data).toBeDefined();
      expect(res.body.data.appSecret).toBe('••••••••••••••••••••••••••••');
    });

    it('POST /admin/refund with nonexistent order returns error', async () => {
      const res = await request(app.getHttpServer())
        .post('/v1/api/admin/refund')
        .set('Authorization', `Bearer ${adminToken}`)
        .send({ orderNo: 'NONEXISTENT', amount: 10 });

      expect([400, 404]).toContain(res.status);
    });

    it('GET /admin/notifications returns list', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/admin/notifications')
        .set('Authorization', `Bearer ${adminToken}`);

      expect(res.status).toBe(200);
    });

    it('GET /admin/audit-logs returns list', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/admin/audit-logs')
        .set('Authorization', `Bearer ${adminToken}`);

      expect(res.status).toBe(200);
    });

    it('GET /admin/settings returns settings', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/admin/settings')
        .set('Authorization', `Bearer ${adminToken}`);

      expect(res.status).toBe(200);
    });
  });

  describe('Gateway auth protection', () => {
    it('POST /gateway/pay without signature returns 401', async () => {
      const res = await request(app.getHttpServer())
        .post('/v1/api/gateway/pay')
        .send({ amount: 100, productName: 'Test', payMethod: 'alipay' });

      expect(res.status).toBe(401);
    });

    it('GET /gateway/query without signature returns 401', async () => {
      const res = await request(app.getHttpServer())
        .get('/v1/api/gateway/query?orderNo=TEST123');

      expect(res.status).toBe(401);
    });

    it('POST /gateway/refund without signature returns 401', async () => {
      const res = await request(app.getHttpServer())
        .post('/v1/api/gateway/refund')
        .send({ orderNo: 'TEST123', amount: 10 });

      expect(res.status).toBe(401);
    });
  });

  describe('Native pay auth protection', () => {
    it('POST /native-pay/confirm without JWT returns 401', async () => {
      const res = await request(app.getHttpServer())
        .post('/v1/api/native-pay/confirm')
        .send({ orderNo: 'TEST123' });

      expect(res.status).toBe(401);
    });
  });

  describe('Health check', () => {
    it('GET /health returns ok', async () => {
      const res = await request(app.getHttpServer()).get('/health');
      expect(res.status).toBe(200);
    });
  });
});
