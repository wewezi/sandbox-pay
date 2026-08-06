import { NonceStore } from './nonce-store.service';

function createService(overrides: Record<string, any> = {}) {
  const defaults = {
    repo: {
      findOne: jest.fn().mockResolvedValue(null),
      save: jest.fn().mockImplementation(async (item: any) => item),
      delete: jest.fn().mockResolvedValue({ affected: 0 }),
    },
  };
  const deps = { ...defaults, ...overrides };
  const service = new NonceStore(deps.repo as any);
  return { service, deps };
}

describe('NonceStore', () => {
  describe('tryConsume', () => {
    it('returns true for a fresh nonce', async () => {
      const { service, deps } = createService();
      deps.repo.findOne.mockResolvedValue(null);

      const result = await service.tryConsume('nonce-1', 60_000);

      expect(result).toBe(true);
      expect(deps.repo.save).toHaveBeenCalled();
    });

    it('returns false for a duplicate nonce within TTL', async () => {
      const { service, deps } = createService();
      const futureDate = new Date(Date.now() + 60000);
      deps.repo.findOne.mockResolvedValue({ nonce: 'nonce-2', expiresAt: futureDate });

      const result = await service.tryConsume('nonce-2', 60_000);

      expect(result).toBe(false);
    });

    it('allows a nonce to be reused after TTL expires', async () => {
      const { service, deps } = createService();
      const pastDate = new Date(Date.now() - 1000);
      deps.repo.findOne.mockResolvedValue({ nonce: 'nonce-3', expiresAt: pastDate });

      const result = await service.tryConsume('nonce-3', 60_000);

      expect(result).toBe(true);
    });

    it('returns false when save fails (concurrent write)', async () => {
      const { service, deps } = createService();
      deps.repo.findOne.mockResolvedValue(null);
      deps.repo.save.mockRejectedValue(new Error('Duplicate entry'));

      const result = await service.tryConsume('nonce-4', 60_000);

      expect(result).toBe(false);
    });

    it('cleans up expired records', async () => {
      const { service, deps } = createService();
      deps.repo.findOne.mockResolvedValue(null);

      await service.tryConsume('nonce-5', 60_000);

      expect(deps.repo.delete).toHaveBeenCalled();
    });
  });
});
