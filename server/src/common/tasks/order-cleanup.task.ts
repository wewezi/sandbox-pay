import { Injectable, Logger } from '@nestjs/common';
import { Cron, CronExpression } from '@nestjs/schedule';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { PaymentOrder, OrderStatus } from '../../entities/payment-order.entity';

@Injectable()
export class OrderCleanupTask {
  private readonly logger = new Logger(OrderCleanupTask.name);

  constructor(
    @InjectRepository(PaymentOrder)
    private orderRepository: Repository<PaymentOrder>,
  ) {}

  @Cron(CronExpression.EVERY_MINUTE)
  async handleCleanup() {
    const now = new Date();
    const result = await this.orderRepository
      .createQueryBuilder()
      .update(PaymentOrder)
      .set({ status: OrderStatus.Expired })
      .where(`"id" IN (
        SELECT "id" FROM "payment_orders"
        WHERE "status" = :status AND "expireAt" < :now
        LIMIT 1000
      )`, { status: OrderStatus.Pending, now })
      .execute();

    if (result.affected && result.affected > 0) {
      this.logger.log(`Closed ${result.affected} expired orders`);
    }
  }
}
