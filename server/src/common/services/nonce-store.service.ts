import { Injectable, Logger } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, LessThan } from 'typeorm';
import { NonceRecord } from '../../entities/nonce-record.entity';

/**
 * 数据库驱动的 nonce 去重存储，用于防重放攻击。
 *
 * 使用数据库替代内存 Map，支持多进程/多实例部署。
 * 过期记录通过 lazy 清理 + 定时清理双机制淘汰。
 */
@Injectable()
export class NonceStore {
  private readonly logger = new Logger(NonceStore.name);

  constructor(
    @InjectRepository(NonceRecord)
    private readonly repo: Repository<NonceRecord>,
  ) {}

  /**
   * 消费一个 nonce。返回 true 表示首次出现（已存储），false 表示重复。
   */
  async tryConsume(key: string, ttlMs: number): Promise<boolean> {
    const now = new Date();

    // 检查是否已存在且未过期
    const existing = await this.repo.findOne({ where: { nonce: key } });
    if (existing && existing.expiresAt > now) {
      return false;
    }

    // 存储新 nonce（已过期的会被覆盖）
    const expiresAt = new Date(now.getTime() + ttlMs);
    try {
      await this.repo.save({ nonce: key, expiresAt });
    } catch {
      // 唯一约束冲突说明并发写入，视为重复
      return false;
    }

    // Lazy 清理：每次调用清理一小批过期记录
    await this.repo.delete({ expiresAt: LessThan(now) });

    return true;
  }
}
