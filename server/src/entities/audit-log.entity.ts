import {
  Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, Index,
} from 'typeorm';

/**
 * Append-only audit trail for sensitive actions (refund, secret reset, etc.).
 *
 * Rows here are written best-effort and must never block the primary
 * business flow — see `AuditService.log()`. Anything sensitive (signatures,
 * raw secrets, full PANs) MUST be redacted before being stored in `detail`.
 */
@Entity('audit_logs')
export class AuditLog {
  @PrimaryGeneratedColumn({ type: 'bigint' })
  id: number;

  /** Stable action verb in snake_case, e.g. 'refund', 'reset_secret'. */
  @Column({ length: 64 })
  @Index()
  action: string;

  /** Free-form actor identifier: 'admin:<username>' or 'merchant:<id>'. */
  @Column({ length: 128 })
  @Index()
  actor: string;

  /** Optional logical target type ('order', 'merchant', 'admin'). */
  @Column({ length: 32, nullable: true })
  targetType?: string;

  /** Optional logical target id (orderNo, merchant id, etc.). */
  @Column({ length: 128, nullable: true })
  @Index()
  targetId?: string;

  @Column({ length: 64, nullable: true })
  ip?: string;

  /** JSON-serialised detail blob. Caller is responsible for redaction. */
  @Column({ type: 'text', nullable: true })
  detail?: string;

  @CreateDateColumn()
  @Index()
  createdAt: Date;
}
