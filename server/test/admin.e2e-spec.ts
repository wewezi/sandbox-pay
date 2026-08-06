import { INestApplication } from '@nestjs/common';
import request from 'supertest';
import { App } from 'supertest/types';
import { createTestApp, generateTestJwt } from './helpers/test-app.helper';
import { JwtService } from '@nestjs/jwt';

describe('Admin (e2e)', () => {
  let app: INestApplication<App>;
  let jwtService: JwtService;
  let token: string;

  beforeAll(async () => {
    const testApp = await createTestApp();
    app = testApp.app;
    jwtService = testApp.jwtService;
    token = generateTestJwt(jwtService);
  });

  afterAll(async () => {
    await app.close();
  });

  describe('auth guard', () => {
    it('rejects requests without JWT', () => {
      return request(app.getHttpServer())
        .get('/api/admin/stats')
        .expect(401);
    });

    it('rejects requests with invalid JWT', () => {
      return request(app.getHttpServer())
        .get('/api/admin/stats')
        .set('Authorization', 'Bearer invalid-token')
        .expect(401);
    });
  });

  describe('GET /api/admin/stats', () => {
    it('returns stats with valid JWT', () => {
      return request(app.getHttpServer())
        .get('/api/admin/stats')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(res.body.data).toBeDefined();
          expect(res.body.data).toHaveProperty('totalAmount');
          expect(res.body.data).toHaveProperty('successCount');
          expect(res.body.data).toHaveProperty('successRate');
        });
    });
  });

  describe('GET /api/admin/transactions', () => {
    it('returns paginated transactions', () => {
      return request(app.getHttpServer())
        .get('/api/admin/transactions')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(res.body.data).toBeDefined();
          expect(res.body.data).toHaveProperty('items');
          expect(res.body.data).toHaveProperty('total');
          expect(res.body.data).toHaveProperty('page');
          expect(res.body.data).toHaveProperty('pageSize');
        });
    });

    it('accepts pagination params', () => {
      return request(app.getHttpServer())
        .get('/api/admin/transactions?page=2&pageSize=10')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.data.page).toBe(2);
          expect(res.body.data.pageSize).toBe(10);
        });
    });
  });

  describe('GET /api/admin/merchant', () => {
    it('returns merchant info with masked secret', () => {
      return request(app.getHttpServer())
        .get('/api/admin/merchant')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(res.body.data).toHaveProperty('appKey');
          expect(res.body.data).toHaveProperty('appSecret');
          expect(res.body.data.appSecret).toContain('••');
          expect(res.body.data.appSecret).not.toContain('enc:v1:');
        });
    });
  });

  describe('POST /api/admin/test-pay', () => {
    it('rejects invalid amount', () => {
      return request(app.getHttpServer())
        .post('/api/admin/test-pay')
        .set('Authorization', `Bearer ${token}`)
        .send({ amount: -1 })
        .expect(400);
    });

    it('rejects zero amount', () => {
      return request(app.getHttpServer())
        .post('/api/admin/test-pay')
        .set('Authorization', `Bearer ${token}`)
        .send({ amount: 0 })
        .expect(400);
    });
  });

  describe('POST /api/admin/refund', () => {
    it('rejects missing orderNo', () => {
      return request(app.getHttpServer())
        .post('/api/admin/refund')
        .set('Authorization', `Bearer ${token}`)
        .send({ amount: 10 })
        .expect(400);
    });

    it('rejects missing amount', () => {
      return request(app.getHttpServer())
        .post('/api/admin/refund')
        .set('Authorization', `Bearer ${token}`)
        .send({ orderNo: 'WP123' })
        .expect(400);
    });
  });

  describe('GET /api/admin/merchants', () => {
    it('returns merchant list', () => {
      return request(app.getHttpServer())
        .get('/api/admin/merchants')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(Array.isArray(res.body.data)).toBe(true);
        });
    });
  });

  describe('POST /api/admin/merchants', () => {
    it('rejects short name', () => {
      return request(app.getHttpServer())
        .post('/api/admin/merchants')
        .set('Authorization', `Bearer ${token}`)
        .send({ name: 'A' })
        .expect(400);
    });

    it('creates merchant with valid name', () => {
      return request(app.getHttpServer())
        .post('/api/admin/merchants')
        .set('Authorization', `Bearer ${token}`)
        .send({ name: 'Test Merchant 2' })
        .expect(201)
        .expect((res) => {
          expect(res.body.data).toHaveProperty('appKey');
          expect(res.body.data).toHaveProperty('appSecret');
          expect(res.body.data.name).toBe('Test Merchant 2');
        });
    });
  });

  describe('GET /api/admin/notifications', () => {
    it('returns notification list', () => {
      return request(app.getHttpServer())
        .get('/api/admin/notifications')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(res.body.data).toHaveProperty('items');
          expect(res.body.data).toHaveProperty('total');
        });
    });
  });

  describe('GET /api/admin/audit-logs', () => {
    it('returns audit log list', () => {
      return request(app.getHttpServer())
        .get('/api/admin/audit-logs')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(res.body.data).toHaveProperty('items');
        });
    });
  });

  describe('GET /api/admin/settings', () => {
    it('returns site settings', () => {
      return request(app.getHttpServer())
        .get('/api/admin/settings')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(typeof res.body.data).toBe('object');
        });
    });
  });

  describe('POST /api/admin/settings', () => {
    it('rejects invalid setting keys', () => {
      return request(app.getHttpServer())
        .post('/api/admin/settings')
        .set('Authorization', `Bearer ${token}`)
        .send({ INVALID_KEY: 'value' })
        .expect(400);
    });

    it('accepts valid setting keys', () => {
      return request(app.getHttpServer())
        .post('/api/admin/settings')
        .set('Authorization', `Bearer ${token}`)
        .send({ FEE_RATE_ALIPAY: '0.01' })
        .expect(201);
    });
  });

  describe('GET /api/admin/reconciliation', () => {
    it('returns reconciliation records', () => {
      return request(app.getHttpServer())
        .get('/api/admin/reconciliation')
        .set('Authorization', `Bearer ${token}`)
        .expect(200)
        .expect((res) => {
          expect(res.body.code).toBe(200);
          expect(res.body.data).toHaveProperty('items');
        });
    });
  });
});
