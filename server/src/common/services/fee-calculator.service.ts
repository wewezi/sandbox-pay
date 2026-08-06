import { Injectable } from '@nestjs/common';
import { SiteSettingsService } from './site-settings.service';

/**
 * Per-channel fee model.
 *
 * All rates are read from site_settings (database).
 * See DEFAULT_SETTINGS in SiteSettingsService for default values.
 */
@Injectable()
export class FeeCalculator {
  constructor(private readonly settings: SiteSettingsService) {}

  calculate(payMethod: string, amountCents: number): { fee: number; channelCost: number; settleAmount: number } {
    const method = (payMethod || '').toUpperCase();
    const feeRate = this.settings.getNumber(`FEE_RATE_${method}`) ?? 0;
    const costRate = this.settings.getNumber(`CHANNEL_COST_${method}`) ?? 0;
    const floor = this.settings.getNumber('FEE_MIN_CENTS') ?? 0;

    const fee = Math.max(Math.round(amountCents * feeRate), Math.round(floor));
    const channelCost = Math.round(amountCents * costRate);
    const settleAmount = Math.max(0, amountCents - fee);
    return { fee, channelCost, settleAmount };
  }
}
