import { ExchangeRateService } from './exchange-rate.service';

function createService(overrides: Record<string, any> = {}) {
  const defaults = {
    httpService: { get: jest.fn() },
    settings: {
      get: jest.fn().mockReturnValue(undefined),
      getNumber: jest.fn().mockReturnValue(undefined),
    },
  };
  const deps = { ...defaults, ...overrides };
  const service = new ExchangeRateService(
    deps.httpService as any,
    deps.settings as any,
  );
  return { service, deps };
}

describe('ExchangeRateService', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('getRate', () => {
    it('fetches rate from open-er-api provider', async () => {
      const { service, deps } = createService();
      deps.settings.get.mockReturnValue('open-er-api');
      deps.settings.getNumber.mockImplementation((key: string) => {
        if (key === 'EXCHANGE_RATE_CACHE_TTL_MS') return 3600000;
        if (key === 'FALLBACK_CNY_TO_USD') return 0.14;
        if (key === 'EXCHANGE_RATE_MARGIN') return 1;
        return undefined;
      });
      const firstValueFromSpy = jest.spyOn(require('rxjs'), 'firstValueFrom')
        .mockResolvedValue({ data: { rates: { USD: 0.138 } } });

      const rate = await service.getRate('CNY', 'USD');

      expect(rate).toBeCloseTo(0.138, 3);
    });

    it('uses fallback when fetch fails', async () => {
      const { service, deps } = createService();
      deps.settings.get.mockReturnValue('open-er-api');
      deps.settings.getNumber.mockImplementation((key: string) => {
        if (key === 'EXCHANGE_RATE_CACHE_TTL_MS') return 3600000;
        if (key === 'FALLBACK_CNY_TO_USD') return 0.14;
        if (key === 'EXCHANGE_RATE_MARGIN') return 1;
        return undefined;
      });
      jest.spyOn(require('rxjs'), 'firstValueFrom').mockRejectedValue(new Error('Network error'));

      const rate = await service.getRate('CNY', 'USD');

      expect(rate).toBeCloseTo(0.14, 2);
    });

    it('applies margin multiplier', async () => {
      const { service, deps } = createService();
      deps.settings.get.mockReturnValue('open-er-api');
      deps.settings.getNumber.mockImplementation((key: string) => {
        if (key === 'EXCHANGE_RATE_CACHE_TTL_MS') return 3600000;
        if (key === 'FALLBACK_CNY_TO_USD') return 0.14;
        if (key === 'EXCHANGE_RATE_MARGIN') return 1.05;
        return undefined;
      });
      jest.spyOn(require('rxjs'), 'firstValueFrom').mockResolvedValue({ data: { rates: { USD: 0.14 } } });

      const rate = await service.getRate('CNY', 'USD');

      expect(rate).toBeCloseTo(0.14 * 1.05, 4);
    });

    it('returns cached rate on subsequent calls', async () => {
      const { service, deps } = createService();
      deps.settings.get.mockReturnValue('open-er-api');
      deps.settings.getNumber.mockImplementation((key: string) => {
        if (key === 'EXCHANGE_RATE_CACHE_TTL_MS') return 3600000;
        if (key === 'FALLBACK_CNY_TO_USD') return 0.14;
        if (key === 'EXCHANGE_RATE_MARGIN') return 1;
        return undefined;
      });
      const firstValueFromSpy = jest.spyOn(require('rxjs'), 'firstValueFrom')
        .mockResolvedValue({ data: { rates: { USD: 0.138 } } });

      await service.getRate('CNY', 'USD');
      await service.getRate('CNY', 'USD');

      expect(firstValueFromSpy).toHaveBeenCalledTimes(1);
    });

    it('uses static provider when configured', async () => {
      const { service, deps } = createService();
      deps.settings.get.mockReturnValue('static');
      deps.settings.getNumber.mockImplementation((key: string) => {
        if (key === 'EXCHANGE_RATE_CACHE_TTL_MS') return 3600000;
        if (key === 'FALLBACK_CNY_TO_USD') return 0.15;
        if (key === 'EXCHANGE_RATE_MARGIN') return 1;
        return undefined;
      });

      const rate = await service.getRate('CNY', 'USD');

      expect(rate).toBeCloseTo(0.15, 2);
      expect(deps.httpService.get).not.toHaveBeenCalled();
    });
  });

  describe('getCnyToUsdRateAsync', () => {
    it('delegates to getRate("CNY", "USD")', async () => {
      const { service, deps } = createService();
      deps.settings.get.mockReturnValue('static');
      deps.settings.getNumber.mockImplementation((key: string) => {
        if (key === 'FALLBACK_CNY_TO_USD') return 0.14;
        return undefined;
      });

      const rate = await service.getCnyToUsdRateAsync();

      expect(rate).toBeCloseTo(0.14, 2);
    });
  });
});
