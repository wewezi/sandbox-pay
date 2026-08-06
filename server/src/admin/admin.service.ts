import { Injectable, Logger, NotFoundException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { PaymentOrder, OrderStatus } from '../entities/payment-order.entity';
import { NotifyQueue, NotifyStatus } from '../entities/notify-queue.entity';
import { AuditLog } from '../entities/audit-log.entity';
import { AuditService } from '../common/services/audit.service';
import { toYuan } from '../common/money';

@Injectable()
export class AdminService {
  private readonly logger = new Logger(AdminService.name);

  constructor(
    @InjectRepository(PaymentOrder)
    private orderRepository: Repository<PaymentOrder>,
    @InjectRepository(NotifyQueue)
    private notifyRepository: Repository<NotifyQueue>,
    @InjectRepository(AuditLog)
    private auditRepository: Repository<AuditLog>,
    private readonly auditService: AuditService,
  ) {}

  /**
   * Stats are computed via SQL aggregation rather than loading rows into memory,
   * so they remain O(1) on the app side regardless of order-table size.
   */
  async getStats(merchantId?: number) {
    const qb = this.orderRepository.createQueryBuilder('o');
    if (merchantId !== undefined) qb.where('o.merchantId = :merchantId', { merchantId });

    const totalCountRaw = await qb.clone().select('COUNT(*)', 'c').getRawOne<{ c: string }>();
    const totalCount = Number(totalCountRaw?.c ?? 0);

    const paidStats = await qb
      .clone()
      .select('COUNT(*)', 'c')
      .addSelect('COALESCE(SUM(o.amount), 0)', 's')
      .andWhere('o.status = :paid', { paid: OrderStatus.Paid })
      .getRawOne<{ c: string; s: string }>();

    const successCount = Number(paidStats?.c ?? 0);
    const totalAmount = Number(paidStats?.s ?? 0);

    // When no orders exist we deliberately surface a neutral placeholder so the
    // dashboard doesn't claim a fake "100% success rate" on an empty account.
    const successRate =
      totalCount > 0 ? ((successCount / totalCount) * 100).toFixed(1) + '%' : '—';

    return {
      totalAmount: `￥${totalAmount.toFixed(2)}`,
      successCount: `${successCount} 笔`,
      successRate,
      rawAmount: totalAmount,
      rawSuccessCount: successCount,
      rawTotalCount: totalCount,
    };
  }

  async getTransactions(opts: {
    merchantId?: number;
    page?: number;
    pageSize?: number;
    status?: string;
    payMethod?: string;
    dateFrom?: string;
    dateTo?: string;
    keyword?: string;
  } = {}) {
    const page = Math.max(1, Number(opts.page) || 1);
    const pageSize = Math.min(200, Math.max(1, Number(opts.pageSize) || 50));

    const qb = this.orderRepository
      .createQueryBuilder('o')
      .orderBy('o.createdAt', 'DESC')
      .skip((page - 1) * pageSize)
      .take(pageSize);
    if (opts.merchantId !== undefined) qb.andWhere('o.merchantId = :merchantId', { merchantId: opts.merchantId });
    if (opts.status) qb.andWhere('o.status = :status', { status: opts.status });
    if (opts.payMethod) qb.andWhere('o.payMethod = :payMethod', { payMethod: opts.payMethod });
    if (opts.dateFrom) qb.andWhere('o.createdAt >= :dateFrom', { dateFrom: new Date(opts.dateFrom) });
    if (opts.dateTo) qb.andWhere('o.createdAt <= :dateTo', { dateTo: new Date(opts.dateTo + 'T23:59:59') });
    if (opts.keyword) {
      qb.andWhere('(o.orderNo LIKE :kw OR o.externalOrderNo LIKE :kw)', { kw: `%${opts.keyword}%` });
    }

    const [orders, total] = await qb.getManyAndCount();

    const items = orders.map(order => {
      let channel = 'ALIPAY';
      if (order.payMethod && order.payMethod.toUpperCase().includes('PAYPAL')) channel = 'PAYPAL';
      else if (order.payMethod && order.payMethod.toUpperCase().includes('WECHAT')) channel = 'WECHAT';
      else if (order.payMethod && order.payMethod.toUpperCase().includes('USDT')) channel = 'USDT';
      else if (order.payMethod && order.payMethod.toUpperCase().includes('NATIVE')) channel = 'NATIVE';
      else if (order.payMethod) channel = order.payMethod.toUpperCase();

      let status = 'PENDING';
      if (order.status === OrderStatus.Paid) status = 'SUCCESS';
      else if (order.status === OrderStatus.Failed || order.status === OrderStatus.Expired) status = 'FAILED';
      else if (order.status === OrderStatus.Refunding) status = 'REFUNDING';
      else if (order.status === OrderStatus.Refunded) status = 'REFUNDED';

      const diffMs = Date.now() - new Date(order.createdAt).getTime();
      const diffMins = Math.floor(diffMs / 60000);
      const diffHours = Math.floor(diffMins / 60);
      const diffDays = Math.floor(diffHours / 24);

      let time = '刚刚';
      if (diffDays > 0) time = `${diffDays} 天前`;
      else if (diffHours > 0) time = `${diffHours} 小时前`;
      else if (diffMins > 0) time = `${diffMins} 分钟前`;

      return {
        id: order.id,
        orderNo: order.orderNo,
        channel,
        amount: toYuan(order.amount),
        refundedAmount: toYuan(order.refundedAmount),
        fee: toYuan(order.fee),
        settleAmount: toYuan(order.settleAmount),
        payMethod: order.payMethod,
        productName: order.productName,
        externalOrderNo: order.externalOrderNo,
        thirdPartyTradeNo: order.thirdPartyTradeNo,
        status,
        time,
        rawDate: order.createdAt,
        payAt: order.payAt,
      };
    });

    return { items, total, page, pageSize };
  }

  async deleteTransaction(orderNo: string, actor: string = 'admin', ip?: string) {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order) {
      throw new NotFoundException('订单不存在');
    }
    await this.orderRepository.delete({ orderNo });

    await this.auditService.log({
      action: 'delete_order',
      actor,
      targetType: 'order',
      targetId: orderNo,
      ip,
      detail: { amount: order.amount, channel: order.payMethod, status: order.status },
    });

    this.logger.log(`Order ${orderNo} deleted by ${actor}`);
    return { success: true };
  }

  // ── Notification Status ────────────────────────────────────────

  async getNotificationStatus(opts: { page?: number; pageSize?: number; status?: string } = {}) {
    const page = Math.max(1, Number(opts.page) || 1);
    const pageSize = Math.min(200, Math.max(1, Number(opts.pageSize) || 50));

    const qb = this.notifyRepository
      .createQueryBuilder('n')
      .orderBy('n.createdAt', 'DESC')
      .skip((page - 1) * pageSize)
      .take(pageSize);
    if (opts.status) qb.andWhere('n.status = :status', { status: Number(opts.status) });

    const [items, total] = await qb.getManyAndCount();
    return {
      items: items.map(n => ({
        id: n.id,
        orderNo: n.orderNo,
        url: n.url,
        status: n.status, // 0=Pending, 1=Success, 2=Failed, 3=Exhausted
        retryCount: n.retryCount,
        lastError: n.lastError,
        lastAttemptAt: n.lastAttemptAt,
        createdAt: n.createdAt,
      })),
      total,
      page,
      pageSize,
    };
  }

  async replayNotification(id: number, actor: string = 'admin', ip?: string) {
    const notify = await this.notifyRepository.findOne({ where: { id } });
    if (!notify) throw new NotFoundException('通知记录不存在');
    notify.status = NotifyStatus.Pending;
    notify.retryCount = 0;
    notify.lastAttemptAt = new Date();
    notify.lastError = undefined;
    await this.notifyRepository.save(notify);

    await this.auditService.log({
      action: 'replay_notification',
      actor,
      targetType: 'notification',
      targetId: String(id),
      ip,
      detail: { orderNo: notify.orderNo },
    });

    return { success: true, id, orderNo: notify.orderNo };
  }

  // ── Data Reset (Sandbox) ───────────────────────────────────────

  async resetData(actor: string = 'admin', ip?: string) {
    const orderCount = await this.orderRepository.count();
    const notifyCount = await this.notifyRepository.count();
    const auditCount = await this.auditRepository.count();

    await this.orderRepository.delete({});
    await this.notifyRepository.delete({});
    await this.auditRepository.delete({});

    this.logger.warn(`Data reset by ${actor}: ${orderCount} orders, ${notifyCount} notifications, ${auditCount} audit logs deleted`);

    return {
      deleted: {
        orders: orderCount,
        notifications: notifyCount,
        auditLogs: auditCount,
      },
    };
  }
}
