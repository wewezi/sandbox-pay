import {
  Entity, PrimaryGeneratedColumn, Column, CreateDateColumn,
  Index, UpdateDateColumn,
} from 'typeorm';

@Entity('merchants')
export class Merchant {
  @PrimaryGeneratedColumn()
  id: number;

  @Column()
  name: string;

  // `unique: true` already creates a unique index — no extra `@Index()` needed.
  @Column({ unique: true })
  appKey: string;

  @Column()
  appSecret: string;

  @Column({ default: true })
  isActive: boolean;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
