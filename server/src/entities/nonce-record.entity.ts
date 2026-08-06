import { Entity, Column, CreateDateColumn, Index } from 'typeorm';

@Entity('nonce_records')
@Index(['nonce', 'expiresAt'])
export class NonceRecord {
  @Column({ length: 255, primary: true })
  nonce: string;

  @Column({ type: 'timestamp' })
  expiresAt: Date;

  @CreateDateColumn()
  createdAt: Date;
}
