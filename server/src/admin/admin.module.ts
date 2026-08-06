import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { AdminController } from './admin.controller';
import { AdminService } from './admin.service';
import { ReconciliationService } from './reconciliation.service';
import { PaymentOrder } from '../entities/payment-order.entity';
import { NotifyQueue } from '../entities/notify-queue.entity';
import { AuditLog } from '../entities/audit-log.entity';
import { ReconciliationRecord } from '../entities/reconciliation-record.entity';
import { PaymentModule } from '../payment/payment.module';

@Module({
  imports: [
    TypeOrmModule.forFeature([PaymentOrder, NotifyQueue, AuditLog, ReconciliationRecord]),
    PaymentModule,
  ],
  controllers: [AdminController],
  providers: [AdminService, ReconciliationService],
  exports: [AdminService, ReconciliationService],
})
export class AdminModule {}
