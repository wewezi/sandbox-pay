import {
  Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, Index, Unique,
} from 'typeorm';
import { moneyColumnTransformer } from '../common/money';

export enum ReconStatus {
  Matched = 'matched',
  AmountMismatch = 'amount_mismatch',
  MissingLocal = 'missing_local',
  MissingUpstream = 'missing_upstream',
}

/**
 * A single row from a daily bill comparison run.
 *
 * Bills from upstream PSPs (Alipay, PayPal) are loaded via `ReconciliationService`
 * either by API pull or by manual CSV upload, then matched against local
 * `payment_orders` for the same date. Discrepancies stay in this table for
 * manual review.
 */
@Entity('reconciliation_records')
@Unique(['provider', 'billDate', 'upstreamTradeNo'])
export class ReconciliationRecord {
  @PrimaryGeneratedColumn({ type: 'bigint' })
  id: number;

  @Column({ length: 32 })
  @Index()
  provider: string; // 'alipay' | 'paypal' | 'native'

  @Column({ type: 'date' })
  @Index()
  billDate: string; // YYYY-MM-DD

  @Column({ length: 64, nullable: true })
  @Index()
  orderNo?: string;

  @Column({ length: 128, nullable: true })
  upstreamTradeNo?: string;

  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  upstreamAmount: number; // cents

  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  localAmount: number; // cents

  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  upstreamFee: number; // cents

  @Column({ length: 32 })
  @Index()
  status: ReconStatus;

  @Column({ type: 'text', nullable: true })
  note?: string;

  @CreateDateColumn()
  createdAt: Date;
}
