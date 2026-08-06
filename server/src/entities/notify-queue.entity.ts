import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, Index } from 'typeorm';

export enum NotifyStatus {
  Pending = 0,
  Success = 1,
  Failed = 2,
  Exhausted = 3,
}

@Entity('notify_queues')
@Index(['status', 'createdAt'])
export class NotifyQueue {
  @PrimaryGeneratedColumn({ type: 'bigint' })
  id: number;

  @Column({ length: 64, nullable: true })
  @Index()
  orderNo?: string;

  @Column({ length: 512 })
  url: string;

  @Column({ type: 'text' })
  body: string;

  @Column({ type: 'text' })
  signature: string;

  @Column({ default: 0 })
  retryCount: number;

  @Column({ type: 'int', default: NotifyStatus.Pending })
  @Index()
  status: NotifyStatus;

  @CreateDateColumn()
  @Index()
  createdAt: Date;

  @Column({ type: 'timestamp', nullable: true })
  @Index()
  lastAttemptAt?: Date;

  @Column({ type: 'text', nullable: true })
  lastError?: string;
}
