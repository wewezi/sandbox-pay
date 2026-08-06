import { Injectable, Logger, OnModuleInit, NotFoundException, BadRequestException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { Merchant } from '../../entities/merchant.entity';
import { EncryptionService } from './encryption.service';
import { AuditService } from './audit.service';
import * as crypto from 'crypto';

/**
 * 商户核心服务 — 提供给 Admin / Gateway / NativePay 等模块共享使用。
 * 从 AdminService 中提取，消除跨模块耦合。
 */
@Injectable()
export class MerchantService implements OnModuleInit {
  private readonly logger = new Logger(MerchantService.name);

  constructor(
    @InjectRepository(Merchant)
    private readonly merchantRepository: Repository<Merchant>,
    private readonly encryptionService: EncryptionService,
    private readonly auditService: AuditService,
  ) {}

  private generateSecret(): string {
    return crypto.createHash('sha256').update(crypto.randomBytes(32)).digest('hex');
  }

  async onModuleInit() {
    try {
      const count = await this.merchantRepository.count();
      if (count === 0) {
        await this.ensureMerchant();
        this.logger.log('✅ 默认沙箱商户数据已同步至数据库');
      } else {
        this.logger.log('✅ 数据库已有商户记录，跳过初始化');
      }
    } catch (err: unknown) {
      this.logger.error('初始化商户数据失败');
    }
  }

  /** 获取或创建默认商户 */
  async ensureMerchant(): Promise<Merchant> {
    let merchant = await this.merchantRepository.findOne({ where: {} });
    if (!merchant) {
      const plain = this.generateSecret();
      merchant = this.merchantRepository.create({
        name: 'Sandbox Pay Merchant',
        appKey: 'sp_sandbox_' + crypto.randomBytes(6).toString('hex'),
        appSecret: this.encryptionService.encrypt(plain),
        isActive: true,
      });
      await this.merchantRepository.save(merchant);
    }
    return merchant;
  }

  /** 查找活跃商户（给 NativePay/Gateway 共用） */
  async findActiveMerchant(): Promise<Merchant> {
    return this.ensureMerchant();
  }

  /** 获取商户信息（密钥脱敏） */
  async getMerchant() {
    const merchant = await this.ensureMerchant();
    return {
      appKey: merchant.appKey,
      appSecret: '••••••••••••••••••••••••••••',
      name: merchant.name,
    };
  }

  /** 重置商户密钥 */
  async resetSecret(actor: string = 'admin', ip?: string) {
    let merchant = await this.merchantRepository.findOne({ where: {} });
    if (!merchant) return this.getMerchant();

    const plain = this.generateSecret();
    merchant.appSecret = this.encryptionService.encrypt(plain);
    await this.merchantRepository.save(merchant);

    await this.auditService.log({
      action: 'reset_secret',
      actor,
      targetType: 'merchant',
      targetId: String(merchant.id),
      ip,
      detail: { appKey: merchant.appKey },
    });

    return { appKey: merchant.appKey, appSecret: plain, name: merchant.name };
  }

  /** 列出全部商户 */
  async listMerchants() {
    const merchants = await this.merchantRepository.find({ order: { id: 'ASC' } });
    return merchants.map(m => ({
      id: m.id,
      name: m.name,
      appKey: m.appKey,
      appSecret: '••••••••••••••••••••••••••••',
      isActive: m.isActive,
      createdAt: m.createdAt,
    }));
  }

  /** 创建商户 */
  async createMerchant(name: string, actor: string = 'admin', ip?: string) {
    if (!name || name.trim().length < 2) {
      throw new BadRequestException('商户名称至少 2 个字符');
    }
    const plain = this.generateSecret();
    const merchant = this.merchantRepository.create({
      name: name.trim(),
      appKey: 'sp_' + crypto.randomBytes(8).toString('hex'),
      appSecret: this.encryptionService.encrypt(plain),
      isActive: true,
    });
    await this.merchantRepository.save(merchant);

    await this.auditService.log({
      action: 'create_merchant',
      actor,
      targetType: 'merchant',
      targetId: String(merchant.id),
      ip,
      detail: { name: merchant.name, appKey: merchant.appKey },
    });

    return { id: merchant.id, name: merchant.name, appKey: merchant.appKey, appSecret: plain, isActive: merchant.isActive };
  }

  /** 更新商户信息 */
  async updateMerchant(id: number, updates: { name?: string }, actor: string = 'admin', ip?: string) {
    const merchant = await this.merchantRepository.findOne({ where: { id } });
    if (!merchant) throw new NotFoundException('商户不存在');
    if (updates.name) merchant.name = updates.name.trim();
    await this.merchantRepository.save(merchant);

    await this.auditService.log({
      action: 'update_merchant',
      actor,
      targetType: 'merchant',
      targetId: String(id),
      ip,
      detail: updates,
    });

    return { id: merchant.id, name: merchant.name, appKey: merchant.appKey, isActive: merchant.isActive };
  }

  /** 启停商户 */
  async toggleMerchantActive(id: number, actor: string = 'admin', ip?: string) {
    const merchant = await this.merchantRepository.findOne({ where: { id } });
    if (!merchant) throw new NotFoundException('商户不存在');
    merchant.isActive = !merchant.isActive;
    await this.merchantRepository.save(merchant);

    await this.auditService.log({
      action: merchant.isActive ? 'activate_merchant' : 'deactivate_merchant',
      actor,
      targetType: 'merchant',
      targetId: String(id),
      ip,
      detail: { isActive: merchant.isActive },
    });

    return { id: merchant.id, name: merchant.name, isActive: merchant.isActive };
  }
}
