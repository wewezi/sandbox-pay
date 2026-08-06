import { SiteSettingsService } from './site-settings.service';

function createService(overrides: Record<string, any> = {}) {
  const defaults = {
    repo: {
      find: jest.fn().mockResolvedValue([]),
      save: jest.fn().mockImplementation(async (items: any) => items),
    },
  };
  const deps = { ...defaults, ...overrides };
  const service = new SiteSettingsService(deps.repo as any);
  return { service, deps };
}

describe('SiteSettingsService', () => {
  describe('onModuleInit', () => {
    it('seeds default settings when DB is empty', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValueOnce([]); // existing check
      deps.repo.find.mockResolvedValueOnce([]); // refresh load

      await service.onModuleInit();

      expect(deps.repo.save).toHaveBeenCalled();
      const savedItems = deps.repo.save.mock.calls[0][0];
      expect(savedItems.length).toBeGreaterThan(0);
      expect(savedItems.some((i: any) => i.key === 'ENABLE_SANDBOX')).toBe(true);
    });

    it('does not seed when DB already has settings', async () => {
      const { service, deps } = createService();
      const existing = [...service.getAllowedKeys()].map(key => ({ key, value: 'configured' }));
      deps.repo.find.mockResolvedValueOnce(existing);
      deps.repo.find.mockResolvedValueOnce(existing);

      await service.onModuleInit();

      expect(deps.repo.save).not.toHaveBeenCalled();
    });
  });

  describe('get', () => {
    it('returns value from cache', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([{ key: 'FEE_RATE_ALIPAY', value: '0.006' }]);
      await service.onModuleInit();

      expect(service.get('FEE_RATE_ALIPAY')).toBe('0.006');
    });

    it('returns undefined for unknown keys', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([]);
      await service.onModuleInit();

      expect(service.get('UNKNOWN_KEY')).toBeUndefined();
    });
  });

  describe('getNumber', () => {
    it('parses numeric values', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([{ key: 'FEE_MIN_CENTS', value: '50' }]);
      await service.onModuleInit();

      expect(service.getNumber('FEE_MIN_CENTS')).toBe(50);
    });

    it('returns undefined for non-numeric values', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([{ key: 'PROVIDER', value: 'open-er-api' }]);
      await service.onModuleInit();

      expect(service.getNumber('PROVIDER')).toBeUndefined();
    });
  });

  describe('getBoolean', () => {
    it('returns true for "true"', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([{ key: 'ENABLE_SANDBOX', value: 'true' }]);
      await service.onModuleInit();

      expect(service.getBoolean('ENABLE_SANDBOX')).toBe(true);
    });

    it('returns false for "false"', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([{ key: 'ENABLE_SANDBOX', value: 'false' }]);
      await service.onModuleInit();

      expect(service.getBoolean('ENABLE_SANDBOX')).toBe(false);
    });
  });

  describe('set / setMany', () => {
    it('updates cache and persists to DB', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([]);
      await service.onModuleInit();

      await service.set('FEE_RATE_ALIPAY', '0.01');

      expect(service.get('FEE_RATE_ALIPAY')).toBe('0.01');
      expect(deps.repo.save).toHaveBeenCalledWith({ key: 'FEE_RATE_ALIPAY', value: '0.01' });
    });

    it('batch updates multiple keys', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([]);
      await service.onModuleInit();

      await service.setMany({ FEE_RATE_ALIPAY: '0.01', FEE_RATE_PAYPAL: '0.05' });

      expect(service.get('FEE_RATE_ALIPAY')).toBe('0.01');
      expect(service.get('FEE_RATE_PAYPAL')).toBe('0.05');
    });
  });

  describe('getAllowedKeys', () => {
    it('returns all default setting keys', async () => {
      const { service, deps } = createService();
      deps.repo.find.mockResolvedValue([]);
      await service.onModuleInit();

      const allowed = service.getAllowedKeys();

      expect(allowed.has('ENABLE_SANDBOX')).toBe(true);
      expect(allowed.has('FEE_RATE_ALIPAY')).toBe(true);
      expect(allowed.has('EXCHANGE_RATE_PROVIDER')).toBe(true);
      expect(allowed.has('UNKNOWN_KEY')).toBe(false);
    });
  });
});
