import { Injectable, Logger, OnModuleInit } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { SiteSetting } from '../../entities/site-setting.entity';

/**
 * 所有业务配置的唯一默认值定义。
 * 首次启动时自动写入 DB，之后全部从 DB 读取。
 */
const DEFAULT_SETTINGS: Record<string, string> = {
  // 沙箱
  ENABLE_SANDBOX: 'true',

  // 限流
  THROTTLE_TTL_MS: '60000',
  THROTTLE_LIMIT: '60',

  // 手续费（小数：0.006 = 0.6%）
  FEE_RATE_ALIPAY: '0.006',
  FEE_RATE_PAYPAL: '0.044',
  FEE_RATE_NATIVE: '0',
  CHANNEL_COST_ALIPAY: '0.006',
  CHANNEL_COST_PAYPAL: '0.044',
  CHANNEL_COST_NATIVE: '0',
  FEE_MIN_CENTS: '0',

  // 汇率
  EXCHANGE_RATE_PROVIDER: 'open-er-api',
  EXCHANGE_RATE_CACHE_TTL_MS: '3600000',
  FALLBACK_CNY_TO_USD: '0.14',
  EXCHANGE_RATE_MARGIN: '1',

  // 支付宝
  ALIPAY_APP_ID: '',
  ALIPAY_SERVER_URL: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do',

  // PayPal
  PAYPAL_CLIENT_ID: '',
  PAYPAL_ENVIRONMENT: 'sandbox',

  // 官方存管
  NATIVE_PAY_QR_URL: '',
  NATIVE_PAY_ACCOUNT_NAME: 'Sandbox Pay Official',
  NATIVE_PAY_ACCOUNT_NO: '',
  NATIVE_PAY_BANK_NAME: '',

  // 邮件
  SMTP_HOST: 'smtp.qq.com',
  SMTP_PORT: '587',
  SMTP_USER: '',
  SMTP_FROM: '',
};

/**
 * 数据库驱动的站点配置服务。
 *
 * 启动时将 DEFAULT_SETTINGS 中缺失的项写入 DB，然后全部加载到内存 Map。
 * 读取走内存，写入走 DB + 刷新内存。
 */
@Injectable()
export class SiteSettingsService implements OnModuleInit {
  private readonly logger = new Logger(SiteSettingsService.name);
  private cache = new Map<string, string>();

  constructor(
    @InjectRepository(SiteSetting)
    private readonly repo: Repository<SiteSetting>,
  ) {}

  async onModuleInit() {
    // 将默认值中 DB 缺失的项写入
    const existing = await this.repo.find({ select: ['key'] });
    const existingKeys = new Set(existing.map(r => r.key));
    const toSeed = Object.entries(DEFAULT_SETTINGS).filter(([key]) => !existingKeys.has(key));
    if (toSeed.length > 0) {
      await this.repo.save(toSeed.map(([key, value]) => ({ key, value })));
      this.logger.log(`✅ 首次写入 ${toSeed.length} 项默认配置到数据库`);
    }
    await this.refresh();
    this.logger.log(`✅ 站点配置已加载 (${this.cache.size} 项)`);
  }

  /** 从 DB 重新加载全部配置到内存 */
  async refresh() {
    const rows = await this.repo.find();
    this.cache.clear();
    for (const row of rows) {
      this.cache.set(row.key, row.value);
    }
  }

  /** 读取配置值（纯 DB，无 env fallback） */
  get(key: string): string | undefined {
    return this.cache.get(key);
  }

  /** 读取为 number，未找到返回 undefined */
  getNumber(key: string): number | undefined {
    const raw = this.cache.get(key);
    if (raw === undefined || raw === '') return undefined;
    const n = Number(raw);
    return Number.isFinite(n) ? n : undefined;
  }

  /** 读取为 boolean，未找到返回 undefined */
  getBoolean(key: string): boolean | undefined {
    const raw = this.cache.get(key);
    if (raw === undefined || raw === '') return undefined;
    return raw === 'true' || raw === '1';
  }

  /** 写入单个配置 */
  async set(key: string, value: string) {
    await this.repo.save({ key, value });
    this.cache.set(key, value);
  }

  /** 批量写入配置 */
  async setMany(entries: Record<string, string>) {
    const rows = Object.entries(entries).map(([key, value]) => ({ key, value }));
    await this.repo.save(rows);
    for (const [key, value] of Object.entries(entries)) {
      this.cache.set(key, value);
    }
  }

  /** 返回允许的配置 key 列表 */
  getAllowedKeys(): Set<string> {
    return new Set(Object.keys(DEFAULT_SETTINGS));
  }

  /** 返回全部配置 */
  getAll(): Record<string, string> {
    const result: Record<string, string> = {};
    for (const [key, value] of this.cache) {
      result[key] = value;
    }
    return result;
  }
}
