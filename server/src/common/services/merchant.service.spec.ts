import { MerchantService } from './merchant.service';
import { NotFoundException, BadRequestException } from '@nestjs/common';

function createMockMerchant(overrides: Partial<any> = {}): any {
  return {
    id: 1,
    name: 'Test Merchant',
    appKey: 'sp_test_abc123',
    appSecret: 'encrypted_secret',
    isActive: true,
    createdAt: new Date(),
    ...overrides,
  };
}

function createService(overrides: Record<string, any> = {}) {
  const defaults = {
    merchantRepository: {
      findOne: jest.fn().mockResolvedValue(null),
      find: jest.fn().mockResolvedValue([]),
      save: jest.fn().mockImplementation(async (m: any) => m),
      create: jest.fn().mockImplementation((m: any) => m),
      count: jest.fn().mockResolvedValue(0),
    },
    encryptionService: {
      encrypt: jest.fn().mockReturnValue('encrypted_value'),
      decrypt: jest.fn().mockReturnValue('decrypted_value'),
    },
    auditService: {
      log: jest.fn().mockResolvedValue(undefined),
    },
  };
  const deps = { ...defaults, ...overrides };
  const service = new MerchantService(
    deps.merchantRepository as any,
    deps.encryptionService as any,
    deps.auditService as any,
  );
  return { service, deps };
}

describe('MerchantService', () => {
  describe('onModuleInit', () => {
    it('creates default merchant when DB is empty', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.count.mockResolvedValue(0);
      deps.merchantRepository.findOne.mockResolvedValue(null);
      deps.merchantRepository.create.mockReturnValue(createMockMerchant());

      await service.onModuleInit();

      expect(deps.merchantRepository.create).toHaveBeenCalled();
      expect(deps.merchantRepository.save).toHaveBeenCalled();
    });

    it('skips initialization when merchants exist', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.count.mockResolvedValue(1);

      await service.onModuleInit();

      expect(deps.merchantRepository.create).not.toHaveBeenCalled();
    });
  });

  describe('getMerchant', () => {
    it('returns merchant with masked secret', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.findOne.mockResolvedValue(createMockMerchant());

      const result = await service.getMerchant();

      expect(result.appKey).toBe('sp_test_abc123');
      expect(result.appSecret).toBe('••••••••••••••••••••••••••••');
    });
  });

  describe('resetSecret', () => {
    it('generates new secret and logs audit', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.findOne.mockResolvedValue(createMockMerchant());

      const result = await service.resetSecret('admin', '127.0.0.1');

      expect(result.appSecret).toBeTruthy();
      expect(result.appSecret).not.toBe('••••••••••••••••••••••••••••');
      expect(deps.encryptionService.encrypt).toHaveBeenCalled();
      expect(deps.auditService.log).toHaveBeenCalledWith(
        expect.objectContaining({ action: 'reset_secret', actor: 'admin' }),
      );
    });
  });

  describe('createMerchant', () => {
    it('creates merchant with generated appKey', async () => {
      const { service, deps } = createService();

      const result = await service.createMerchant('New Merchant', 'admin', '127.0.0.1');

      expect(result.name).toBe('New Merchant');
      expect(result.appKey).toBeTruthy();
      expect(result.appSecret).toBeTruthy();
      expect(deps.auditService.log).toHaveBeenCalled();
    });

    it('rejects names shorter than 2 characters', async () => {
      const { service } = createService();

      await expect(service.createMerchant('A', 'admin')).rejects.toThrow(BadRequestException);
    });
  });

  describe('updateMerchant', () => {
    it('updates merchant name', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.findOne.mockResolvedValue(createMockMerchant());

      const result = await service.updateMerchant(1, { name: 'Updated Name' }, 'admin');

      expect(result.name).toBe('Updated Name');
    });

    it('throws when merchant not found', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.findOne.mockResolvedValue(null);

      await expect(service.updateMerchant(999, { name: 'Test' }, 'admin')).rejects.toThrow(NotFoundException);
    });
  });

  describe('toggleMerchantActive', () => {
    it('toggles active status', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.findOne.mockResolvedValue(createMockMerchant({ isActive: true }));

      const result = await service.toggleMerchantActive(1, 'admin');

      expect(result.isActive).toBe(false);
    });
  });

  describe('listMerchants', () => {
    it('returns list with masked secrets', async () => {
      const { service, deps } = createService();
      deps.merchantRepository.find.mockResolvedValue([createMockMerchant()]);

      const result = await service.listMerchants();

      expect(result).toHaveLength(1);
      expect(result[0].appSecret).toBe('••••••••••••••••••••••••••••');
    });
  });
});
