import { Injectable, Logger } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { AuditLog } from '../../entities/audit-log.entity';
import { errMessage } from '../util/error';
import { redact } from '../logging/redact';

export interface AuditEntry {
  action: string;
  actor: string;
  targetType?: string;
  targetId?: string;
  ip?: string;
  detail?: Record<string, unknown>;
}

/**
 * Centralised, fire-and-forget audit logger. Errors are swallowed so the
 * audit trail never blocks the primary business flow — auditing is a
 * supporting concern, not a transactional one.
 */
@Injectable()
export class AuditService {
  private readonly logger = new Logger(AuditService.name);

  constructor(
    @InjectRepository(AuditLog)
    private readonly auditRepository: Repository<AuditLog>,
  ) {}

  async log(entry: AuditEntry): Promise<void> {
    try {
      // Auto-redact sensitive fields before persisting to the database.
      const safeDetail = entry.detail ? redact(entry.detail) : undefined;
      const row = this.auditRepository.create({
        action: entry.action,
        actor: entry.actor,
        targetType: entry.targetType,
        targetId: entry.targetId,
        ip: entry.ip,
        detail: safeDetail ? JSON.stringify(safeDetail) : undefined,
      });
      await this.auditRepository.save(row);
    } catch (err: unknown) {
      // Never propagate audit failures.
      this.logger.error(`Audit log failed (${entry.action}): ${errMessage(err)}`);
    }
  }

  /**
   * Paginated reader for the admin UI. Newest rows first.
   */
  async list(opts: {
    action?: string;
    actor?: string;
    targetId?: string;
    page?: number;
    pageSize?: number;
  } = {}) {
    const page = Math.max(1, Number(opts.page) || 1);
    const pageSize = Math.min(200, Math.max(1, Number(opts.pageSize) || 50));

    const qb = this.auditRepository
      .createQueryBuilder('a')
      .orderBy('a.createdAt', 'DESC')
      .skip((page - 1) * pageSize)
      .take(pageSize);
    if (opts.action) qb.andWhere('a.action = :a', { a: opts.action });
    if (opts.actor) qb.andWhere('a.actor LIKE :u', { u: `%${opts.actor}%` });
    if (opts.targetId) qb.andWhere('a.targetId = :t', { t: opts.targetId });

    const [items, total] = await qb.getManyAndCount();
    return {
      items: items.map(r => ({
        id: r.id,
        action: r.action,
        actor: r.actor,
        targetType: r.targetType,
        targetId: r.targetId,
        ip: r.ip,
        detail: r.detail ? safeParse(r.detail) : undefined,
        createdAt: r.createdAt,
      })),
      total,
      page,
      pageSize,
    };
  }
}

function safeParse(s: string): unknown {
  try { return JSON.parse(s); } catch (_) { return s; }
}
