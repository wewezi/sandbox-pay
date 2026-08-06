import { Injectable, Logger } from '@nestjs/common';
import { HttpService } from '@nestjs/axios';
import { firstValueFrom } from 'rxjs';
import { SiteSettingsService } from './site-settings.service';
import { errMessage } from '../util/error';

interface CachedRate {
  value: number;
  expiresAt: number;
}

/**
 * Fetches live CNY→USD exchange rate from a public provider and caches it in
 * memory. Falls back to a configurable static rate if all upstream calls fail.
 *
 * All config is read from site_settings (database).
 * See DEFAULT_SETTINGS in SiteSettingsService for default values.
 */
@Injectable()
export class ExchangeRateService {
  private readonly logger = new Logger(ExchangeRateService.name);
  private readonly cache = new Map<string, CachedRate>();

  constructor(
    private readonly httpService: HttpService,
    private readonly settings: SiteSettingsService,
  ) {}

  async getCnyToUsdRateAsync(): Promise<number> {
    return this.getRate('CNY', 'USD');
  }

  async getRate(from: string, to: string): Promise<number> {
    const key = `${from}->${to}`;
    const cached = this.cache.get(key);
    const now = Date.now();
    if (cached && cached.expiresAt > now) return cached.value;

    const provider = this.settings.get('EXCHANGE_RATE_PROVIDER') ?? 'open-er-api';
    const cacheTtlMs = this.settings.getNumber('EXCHANGE_RATE_CACHE_TTL_MS') ?? 3600000;
    const fallback = this.settings.getNumber('FALLBACK_CNY_TO_USD') ?? 0.14;
    const margin = this.settings.getNumber('EXCHANGE_RATE_MARGIN') ?? 1;

    let rate: number | null = null;
    if (provider === 'open-er-api') rate = await this.fetchOpenErApi(from, to);

    if (rate == null || !Number.isFinite(rate) || rate <= 0) {
      this.logger.warn(`Exchange rate fetch failed for ${key}, using fallback ${fallback}`);
      rate = fallback;
    }

    const effective = rate * margin;
    this.cache.set(key, { value: effective, expiresAt: now + cacheTtlMs });
    return effective;
  }

  private async fetchOpenErApi(from: string, to: string): Promise<number | null> {
    try {
      const url = `https://open.er-api.com/v6/latest/${encodeURIComponent(from)}`;
      const res = await firstValueFrom(this.httpService.get(url, { timeout: 5000 }));
      const rate = res?.data?.rates?.[to];
      return typeof rate === 'number' ? rate : null;
    } catch (err: unknown) {
      this.logger.warn(`open.er-api fetch error: ${errMessage(err)}`);
      return null;
    }
  }
}
