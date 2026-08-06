import { FeeCalculator } from './fee-calculator.service';

function createCalculator(env: Record<string, string> = {}): FeeCalculator {
  const settings = {
    get: (key: string) => env[key],
    getNumber: (key: string) => {
      const raw = env[key];
      if (raw == null || raw === '') return undefined;
      const n = Number(raw);
      return Number.isFinite(n) ? n : undefined;
    },
    getBoolean: (_key: string) => undefined,
  } as any;
  return new FeeCalculator(settings);
}

describe('FeeCalculator', () => {
  describe('calculate', () => {
    it('calculates alipay fee at 0.6% when configured', () => {
      const calc = createCalculator({
        FEE_RATE_ALIPAY: '0.006',
        CHANNEL_COST_ALIPAY: '0.006',
        FEE_MIN_CENTS: '0',
      });
      const result = calc.calculate('alipay', 10000);

      expect(result.fee).toBe(60);
      expect(result.channelCost).toBe(60);
      expect(result.settleAmount).toBe(9940);
    });

    it('calculates paypal fee at 4.4% when configured', () => {
      const calc = createCalculator({
        FEE_RATE_PAYPAL: '0.044',
        CHANNEL_COST_PAYPAL: '0.044',
        FEE_MIN_CENTS: '0',
      });
      const result = calc.calculate('paypal', 10000);

      expect(result.fee).toBe(440);
      expect(result.channelCost).toBe(440);
      expect(result.settleAmount).toBe(9560);
    });

    it('returns 0 when no rate configured', () => {
      const calc = createCalculator();
      const result = calc.calculate('native', 10000);

      expect(result.fee).toBe(0);
      expect(result.channelCost).toBe(0);
      expect(result.settleAmount).toBe(10000);
    });

    it('applies minimum fee floor', () => {
      const calc = createCalculator({ FEE_RATE_NATIVE: '0', FEE_MIN_CENTS: '50' });
      const result = calc.calculate('native', 100);

      expect(result.fee).toBe(50);
      expect(result.settleAmount).toBe(50);
    });

    it('margin = fee - channelCost represents gross profit', () => {
      const calc = createCalculator({
        FEE_RATE_PAYPAL: '0.05',
        CHANNEL_COST_PAYPAL: '0.03',
        FEE_MIN_CENTS: '0',
      });
      const result = calc.calculate('paypal', 10000);

      expect(result.fee).toBe(500);
      expect(result.channelCost).toBe(300);
      expect(result.fee - result.channelCost).toBe(200);
    });
  });
});
