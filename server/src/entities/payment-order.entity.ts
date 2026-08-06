import {
  Entity, PrimaryColumn, Column, CreateDateColumn,
  Index, ManyToOne, JoinColumn, UpdateDateColumn,
} from 'typeorm';
import { Merchant } from './merchant.entity';
import { moneyColumnTransformer } from '../common/money';

// All money columns below use `moneyColumnTransformer` so the DB stores yuan as
// decimal(p,2) (human-readable) while in-memory values are integer cents. This
// avoids floating-point arithmetic on amounts without requiring data migration.

export enum OrderStatus {
  Expired = -1,
  Pending = 0,
  Paid = 1,
  Failed = 2,
  Refunding = 3,
  Refunded = 4,
}

@Entity('payment_orders')
@Index(['merchantId', 'status'])
@Index(['merchantId', 'createdAt'])
export class PaymentOrder {
  @PrimaryColumn({ length: 32 })
  id: string;

  // `unique: true` already creates a unique index — no extra `@Index()` needed.
  @Column({ length: 64, unique: true })
  orderNo: string;

  @Column({ type: 'decimal', precision: 18, scale: 2, transformer: moneyColumnTransformer })
  amount: number; // cents

  @Column()
  productName: string;

  @Column({ length: 32 })
  payMethod: string;

  @Column({ type: 'int', default: OrderStatus.Pending })
  @Index()
  status: OrderStatus;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;

  @Column({ type: 'timestamp' })
  @Index()
  expireAt: Date;

  @Column({ type: 'timestamp', nullable: true })
  payAt?: Date;

  @Column({ type: 'decimal', precision: 18, scale: 10, nullable: true })
  exchangeRate?: number;

  @Column({ type: 'decimal', precision: 18, scale: 2, nullable: true, transformer: moneyColumnTransformer })
  foreignAmount?: number; // minor units of `foreignCurrency`

  @Column({ length: 10, nullable: true })
  foreignCurrency?: string;

  @Column({ length: 64, nullable: true })
  @Index()
  externalOrderNo?: string;

  @Column({ length: 128, nullable: true })
  thirdPartyTradeNo?: string;

  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  refundedAmount: number; // cents

  @Column({ length: 128, nullable: true })
  refundTradeNo?: string;

  @Column({ type: 'timestamp', nullable: true })
  refundAt?: Date;

  // Channel fee charged by the upstream PSP (Alipay/PayPal).
  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  channelCost: number; // cents

  // Fee charged by Sandbox Pay to the merchant (revenue).
  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  fee: number; // cents

  // Amount payable to the merchant after deducting `fee`. Computed at markPaid time.
  @Column({ type: 'decimal', precision: 18, scale: 2, default: 0, transformer: moneyColumnTransformer })
  settleAmount: number; // cents

  @Column({ length: 512, nullable: true })
  returnUrl?: string;

  @Column({ length: 512, nullable: true })
  notifyUrl?: string;

  @Column({ nullable: true })
  @Index()
  merchantId?: number;

  @Column({ name: 'userId', default: 1 })
  userId: number;

  @ManyToOne(() => Merchant, { onDelete: 'SET NULL', nullable: true })
  @JoinColumn({ name: 'merchantId' })
  merchant?: Merchant;
}
