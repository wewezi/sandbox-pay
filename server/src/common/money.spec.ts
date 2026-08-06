import { yuanStringToCents, toYuan, toYuanString, moneyColumnTransformer } from './money';

describe('money helpers', () => {
  describe('yuanStringToCents', () => {
    it('converts yuan string to cents', () => {
      expect(yuanStringToCents('10.00')).toBe(1000);
      expect(yuanStringToCents('0.01')).toBe(1);
      expect(yuanStringToCents('99.99')).toBe(9999);
    });

    it('converts yuan number to cents', () => {
      expect(yuanStringToCents(10)).toBe(1000);
      expect(yuanStringToCents(0.01)).toBe(1);
    });

    it('handles null/undefined/empty', () => {
      expect(yuanStringToCents(null)).toBe(0);
      expect(yuanStringToCents(undefined)).toBe(0);
      expect(yuanStringToCents('')).toBe(0);
    });

    it('rounds correctly to avoid floating-point drift', () => {
      expect(yuanStringToCents('0.10')).toBe(10);
      expect(yuanStringToCents('0.29')).toBe(29);
      expect(yuanStringToCents('1.005')).toBe(100); // Math.round(100.5) = 100 in JS
    });
  });

  describe('toYuan', () => {
    it('converts cents to yuan', () => {
      expect(toYuan(1000)).toBe(10);
      expect(toYuan(1)).toBe(0.01);
      expect(toYuan(9999)).toBe(99.99);
    });

    it('handles null/undefined', () => {
      expect(toYuan(null)).toBe(0);
      expect(toYuan(undefined)).toBe(0);
    });
  });

  describe('toYuanString', () => {
    it('returns fixed-2 string', () => {
      expect(toYuanString(1000)).toBe('10.00');
      expect(toYuanString(1)).toBe('0.01');
      expect(toYuanString(0)).toBe('0.00');
    });

    it('handles null', () => {
      expect(toYuanString(null)).toBe('0.00');
    });
  });

  describe('moneyColumnTransformer', () => {
    it('to: cents → yuan string for DB', () => {
      expect(moneyColumnTransformer.to(1000)).toBe('10.00');
      expect(moneyColumnTransformer.to(1)).toBe('0.01');
      expect(moneyColumnTransformer.to(null)).toBeNull();
    });

    it('from: yuan string → cents for code', () => {
      expect(moneyColumnTransformer.from('10.00')).toBe(1000);
      expect(moneyColumnTransformer.from('0.01')).toBe(1);
      expect(moneyColumnTransformer.from(null)).toBeNull();
    });

    it('roundtrip preserves value', () => {
      const amounts = [1, 10, 100, 9999, 100000];
      for (const cents of amounts) {
        const dbValue = moneyColumnTransformer.to(cents);
        expect(moneyColumnTransformer.from(dbValue!)).toBe(cents);
      }
    });
  });
});
