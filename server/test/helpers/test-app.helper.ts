import { Test, TestingModule } from '@nestjs/testing';
import { INestApplication, ValidationPipe } from '@nestjs/common';
import { getRepositoryToken } from '@nestjs/typeorm';
import { JwtService } from '@nestjs/jwt';
import { ConfigService } from '@nestjs/config';
import { AppModule } from '../../src/app.module';
import { User } from '../../src/entities/user.entity';
import { Merchant } from '../../src/entities/merchant.entity';
import { PaymentOrder, OrderStatus } from '../../src/entities/payment-order.entity';
import { NotifyQueue } from '../../src/entities/notify-queue.entity';
import { AuditLog } from '../../src/entities/audit-log.entity';
import { ReconciliationRecord } from '../../src/entities/reconciliation-record.entity';
import { AllExceptionsFilter } from '../../src/common/filters/all-exceptions.filter';
import { TransformInterceptor } from '../../src/common/interceptors/transform.interceptor';
import { EncryptionService } from '../../src/common/services/encryption.service';

/** A minimal mock Repository that supports the methods used by the app. */
function createMockRepo(initialData: Record<string, any>[] = []) {
  const data = [...initialData];
  return {
    find: jest.fn().mockResolvedValue(data),
    findOne: jest.fn().mockImplementation(({ where }: any) => {
      const entry = data.find((d) =>
        Object.entries(where).every(([k, v]) => d[k] === v),
      );
      return Promise.resolve(entry ?? null);
    }),
    create: jest.fn().mockImplementation((dto: any) => ({ id: data.length + 1, ...dto })),
    save: jest.fn().mockImplementation((entity: any) => {
      if (!entity.id) entity.id = data.length + 1;
      const idx = data.findIndex((d) => d.id === entity.id);
      if (idx >= 0) data[idx] = entity;
      else data.push(entity);
      return Promise.resolve(entity);
    }),
    delete: jest.fn().mockResolvedValue({ affected: 1 }),
    count: jest.fn().mockResolvedValue(data.length),
    createQueryBuilder: jest.fn().mockReturnValue({
      where: jest.fn().mockReturnThis(),
      andWhere: jest.fn().mockReturnThis(),
      select: jest.fn().mockReturnThis(),
      addSelect: jest.fn().mockReturnThis(),
      orderBy: jest.fn().mockReturnThis(),
      skip: jest.fn().mockReturnThis(),
      take: jest.fn().mockReturnThis(),
      clone: jest.fn().mockReturnThis(),
      getRawOne: jest.fn().mockResolvedValue({ c: '0', s: '0' }),
      getManyAndCount: jest.fn().mockResolvedValue([data, data.length]),
      getMany: jest.fn().mockResolvedValue(data),
    }),
    _data: data,
  };
}

/**
 * Create a fully-wired NestJS testing app with mocked database and
 * external services. Returns the app + repos for test-specific setup.
 */
export async function createTestApp() {
  const mockUserRepo = createMockRepo([
    { id: 1, username: 'testadmin', password: 'hashed', role: 'admin' },
  ]);
  const mockMerchantRepo = createMockRepo([
    {
      id: 1,
      name: 'Test Merchant',
      appKey: 'sp_test_key',
      appSecret: 'enc:v1:test-encrypted-secret',
      isActive: true,
    },
  ]);
  const mockOrderRepo = createMockRepo();
  const mockNotifyRepo = createMockRepo();
  const mockAuditRepo = createMockRepo();
  const mockReconRepo = createMockRepo();

  const moduleFixture: TestingModule = await Test.createTestingModule({
    imports: [AppModule],
  })
    .overrideProvider(getRepositoryToken(User))
    .useValue(mockUserRepo)
    .overrideProvider(getRepositoryToken(Merchant))
    .useValue(mockMerchantRepo)
    .overrideProvider(getRepositoryToken(PaymentOrder))
    .useValue(mockOrderRepo)
    .overrideProvider(getRepositoryToken(NotifyQueue))
    .useValue(mockNotifyRepo)
    .overrideProvider(getRepositoryToken(AuditLog))
    .useValue(mockAuditRepo)
    .overrideProvider(getRepositoryToken(ReconciliationRecord))
    .useValue(mockReconRepo)
    .overrideProvider(EncryptionService)
    .useValue({
      encrypt: jest.fn().mockImplementation((plain: string) => `enc:v1:mock-${plain}`),
      decrypt: jest.fn().mockImplementation((val: string) => {
        // If it looks like our mock format, extract the plaintext; otherwise return as-is
        if (val.startsWith('enc:v1:mock-')) return val.slice('enc:v1:mock-'.length);
        if (val.startsWith('enc:v1:')) return 'test-secret';
        return val;
      }),
    })
    .compile();

  const app = moduleFixture.createNestApplication();
  app.useGlobalPipes(new ValidationPipe({ whitelist: true, transform: true, forbidNonWhitelisted: true }));
  app.useGlobalFilters(new AllExceptionsFilter());
  app.useGlobalInterceptors(new TransformInterceptor());
  await app.init();

  const jwtService = app.get(JwtService);

  return {
    app,
    jwtService,
    repos: {
      user: mockUserRepo,
      merchant: mockMerchantRepo,
      order: mockOrderRepo,
      notify: mockNotifyRepo,
      audit: mockAuditRepo,
      recon: mockReconRepo,
    },
  };
}

/** Generate a valid JWT token for test admin. */
export function generateTestJwt(jwtService: JwtService): string {
  return jwtService.sign({ sub: 1, username: 'testadmin', role: 'admin' });
}
