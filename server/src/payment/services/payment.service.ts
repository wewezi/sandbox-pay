import { Injectable, NotFoundException, BadRequestException, ConflictException, Logger } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { PaymentOrder, OrderStatus } from '../../entities/payment-order.entity';
import { Merchant } from '../../entities/merchant.entity';
import { CreatePaymentDto, RefundDto } from '../dto/payment.dto';
import { OrderNumberGenerator } from '../../common/services/order-number-generator.service';
import { NotifyService, PaymentNotification } from '../../common/services/notify.service';
import { EncryptionService } from '../../common/services/encryption.service';
import { FeeCalculator } from '../../common/services/fee-calculator.service';
import { PaymentGateway } from '../payment.gateway';
import { toYuan, yuanStringToCents } from '../../common/money';
import { v4 as uuidv4 } from 'uuid';

@Injectable()
export class PaymentService {
  private readonly logger = new Logger(PaymentService.name);

  constructor(
    @InjectRepository(PaymentOrder)
    private orderRepository: Repository<PaymentOrder>,
    @InjectRepository(Merchant)
    private merchantRepository: Repository<Merchant>,
    private orderNumberGenerator: OrderNumberGenerator,
    private notifyService: NotifyService,
    private encryptionService: EncryptionService,
    private feeCalculator: FeeCalculator,
    private paymentGateway: PaymentGateway,
  ) {}

  async createOrder(merchantId: number, dto: CreatePaymentDto) {
    // DTO carries yuan; convert to integer cents at the boundary.
    const amountCents = yuanStringToCents(dto.amount);

    // Idempotency: ONLY de-duplicate on (merchantId, externalOrderNo).
    if (dto.externalOrderNo) {
      const existingOrder = await this.orderRepository.findOne({
        where: {
          merchantId,
          externalOrderNo: dto.externalOrderNo,
        },
        order: { createdAt: 'DESC' },
      });

      if (existingOrder) {
        // amount is already cents (via column transformer) — compare exactly.
        if (existingOrder.amount !== amountCents) {
          // Same idempotency key, different amount: caller bug, not a missing
          // resource. 400 is semantically correct here.
          throw new BadRequestException(
            `externalOrderNo ${dto.externalOrderNo} already exists with a different amount`,
          );
        }
        if (existingOrder.status === OrderStatus.Pending && existingOrder.expireAt > new Date()) {
          if (existingOrder.payMethod !== dto.payMethod) {
            existingOrder.payMethod = dto.payMethod;
            await this.orderRepository.save(existingOrder);
          }
          return this.buildOrderResult(existingOrder);
        }
        return this.buildOrderResult(existingOrder);
      }
    }

    const expireAt = new Date();
    expireAt.setMinutes(expireAt.getMinutes() + 30);

    const order = this.orderRepository.create({
      id: uuidv4().replace(/-/g, ''),
      orderNo: this.orderNumberGenerator.create('PAY'),
      amount: amountCents,
      productName: dto.productName,
      payMethod: dto.payMethod,
      externalOrderNo: dto.externalOrderNo,
      returnUrl: dto.returnUrl,
      notifyUrl: dto.notifyUrl,
      merchantId,
      // userId: 0 = unassigned (no end-user system yet)
      userId: 0,
      status: OrderStatus.Pending,
      expireAt,
    });

    await this.orderRepository.save(order);
    return this.buildOrderResult(order);
  }

  async queryOrder(orderNo: string) {
    const order = await this.orderRepository.findOne({
      where: { orderNo },
      relations: ['merchant'],
    });
    if (!order) throw new NotFoundException('订单不存在');

    return {
      orderNo: order.orderNo,
      externalOrderNo: order.externalOrderNo,
      amount: toYuan(order.amount),
      status: order.status,
      payMethod: order.payMethod,
      productName: order.productName,
      returnUrl: order.returnUrl,
      payAt: order.payAt,
      thirdPartyTradeNo: order.thirdPartyTradeNo,
      createdAt: order.createdAt,
      expireAt: order.expireAt,
    };
  }

  async queryByMerchant(merchantId: number, orderNo: string) {
    const order = await this.orderRepository.findOne({
      where: { orderNo, merchantId },
    });
    if (!order) throw new NotFoundException('订单不存在');

    return {
      orderNo: order.orderNo,
      externalOrderNo: order.externalOrderNo,
      amount: toYuan(order.amount),
      refundedAmount: toYuan(order.refundedAmount),
      fee: toYuan(order.fee),
      settleAmount: toYuan(order.settleAmount),
      status: order.status,
      payMethod: order.payMethod,
      productName: order.productName,
      payAt: order.payAt,
      thirdPartyTradeNo: order.thirdPartyTradeNo,
      createdAt: order.createdAt,
      expireAt: order.expireAt,
    };
  }

  /**
   * Mark an order as paid after a gateway-confirmed payment.
   *
   * Callers MUST pass the actual amount and currency reported by the upstream
   * gateway. The values are compared against the original order to defend
   * against tampered / forged callbacks that report success for the wrong amount.
   */
  async markPaid(
    orderNo: string,
    thirdPartyTradeNo: string | undefined,
    paidAmountCents: number,
    paidCurrency: string = 'CNY',
  ) {
    // 事务 + 行锁防止并发回调导致重复入账
    const order = await this.orderRepository.manager.transaction(async (txMgr) => {
      const order = await txMgr.findOne(PaymentOrder, {
        where: { orderNo },
        relations: ['merchant'],
        lock: { mode: 'pessimistic_write' },
      });
      if (!order) throw new NotFoundException('订单不存在');
      if (order.status !== OrderStatus.Pending) {
        this.logger.warn(`Order ${orderNo} already in status ${order.status}, skip`);
        return order;
      }

      const expectedCents = order.amount;
      const expectedForeignCents = order.foreignAmount ?? null;
      const tolerance = 1;

      let amountValid = false;
      const currency = (paidCurrency || 'CNY').toUpperCase();
      if (currency === 'CNY') {
        amountValid = Math.abs(paidAmountCents - expectedCents) <= tolerance;
      } else if (expectedForeignCents != null && currency === (order.foreignCurrency || '').toUpperCase()) {
        amountValid = Math.abs(paidAmountCents - expectedForeignCents) <= tolerance;
      }

      if (!amountValid) {
        this.logger.error(
          `Amount mismatch on order ${orderNo}: gateway reported ${paidAmountCents} ${currency} (cents), ` +
            `expected ${expectedCents} CNY / ${expectedForeignCents ?? 'n/a'} ${order.foreignCurrency ?? ''}`,
        );
        throw new ConflictException(
          `订单 ${orderNo} 金额不匹配，拒绝入账`,
        );
      }

      const fees = this.feeCalculator.calculate(order.payMethod, order.amount);
      order.fee = fees.fee;
      order.channelCost = fees.channelCost;
      order.settleAmount = fees.settleAmount;

      order.status = OrderStatus.Paid;
      order.payAt = new Date();
      order.thirdPartyTradeNo = thirdPartyTradeNo;
      await txMgr.save(PaymentOrder, order);

      this.logger.log(
        `Order ${orderNo} marked as paid (${paidAmountCents} ${paidCurrency} cents); ` +
          `fee=${fees.fee} cost=${fees.channelCost} settle=${fees.settleAmount}`,
      );
      return order;
    });

    // 事务提交后再发副作用（WebSocket + 通知队列）
    if (order.status === OrderStatus.Paid) {
      this.paymentGateway.notifyPaymentStatus(orderNo, 'paid');

      if (order.notifyUrl && order.merchant) {
        const notification: PaymentNotification = {
          orderNo: order.orderNo,
          externalOrderNo: order.externalOrderNo,
          status: 'paid',
          amount: toYuan(order.amount),
          payMethod: order.payMethod,
          payAt: order.payAt!.toISOString(),
          thirdPartyTradeNo,
        };
        const secret = this.encryptionService.decrypt(order.merchant.appSecret);
        await this.notifyService.enqueueNotification(order.notifyUrl, notification, secret);
      }
    }

    return order;
  }

  /**
   * Mark a paid order as fully or partially refunded. Caller must have already
   * executed the refund on the upstream gateway and obtained a refundTradeNo.
   *
   * Idempotent on (orderNo, refundTradeNo) — same trade-no reapplied is a no-op.
   */
  async markRefunded(
    orderNo: string,
    refundedCents: number,
    refundTradeNo: string,
    merchantIdForCheck?: number,
  ) {
    // 事务 + 行锁防止并发退款导致重复入账
    const order = await this.orderRepository.manager.transaction(async (txMgr) => {
      const order = await txMgr.findOne(PaymentOrder, {
        where: { orderNo },
        relations: ['merchant'],
        lock: { mode: 'pessimistic_write' },
      });
      if (!order) throw new NotFoundException('订单不存在');
      if (merchantIdForCheck !== undefined && order.merchantId !== merchantIdForCheck) {
        throw new NotFoundException('订单不存在');
      }

      // Idempotency: same refund trade-no replayed → no-op once already settled.
      if (order.refundTradeNo === refundTradeNo && order.status === OrderStatus.Refunded) {
        return order;
      }

      if (order.status !== OrderStatus.Paid && order.status !== OrderStatus.Refunding) {
        throw new ConflictException(`订单状态不允许退款: ${order.status}`);
      }

      const newRefunded = (order.refundedAmount || 0) + refundedCents;
      if (newRefunded - order.amount > 1) {
        throw new BadRequestException('退款累计金额超过订单金额');
      }

      order.refundedAmount = newRefunded;
      order.refundTradeNo = refundTradeNo;
      order.refundAt = new Date();
      if (Math.abs(newRefunded - order.amount) <= 1) {
        order.status = OrderStatus.Refunded;
      }
      await txMgr.save(PaymentOrder, order);

      this.logger.log(
        `Order ${orderNo} refund applied: ${refundedCents} cents (total refunded ${newRefunded}, status ${order.status})`,
      );
      return order;
    });

    // 事务提交后再发副作用
    this.paymentGateway.notifyPaymentStatus(orderNo, order.status === OrderStatus.Refunded ? 'refunded' : 'partial_refunded');

    if (order.notifyUrl && order.merchant) {
      const notification: PaymentNotification = {
        orderNo: order.orderNo,
        externalOrderNo: order.externalOrderNo,
        status: order.status === OrderStatus.Refunded ? 'refunded' : 'partial_refunded',
        amount: toYuan(order.amount),
        payMethod: order.payMethod,
        payAt: (order.payAt ?? new Date()).toISOString(),
        thirdPartyTradeNo: refundTradeNo,
      };
      const secret = this.encryptionService.decrypt(order.merchant.appSecret);
      await this.notifyService.enqueueNotification(order.notifyUrl, notification, secret);
    }

    return order;
  }

  /**
   * Mark a refund as in-flight (gateway accepted but final state pending).
   */
  async markRefunding(orderNo: string) {
    return this.orderRepository.manager.transaction(async (txMgr) => {
      const order = await txMgr.findOne(PaymentOrder, {
        where: { orderNo },
        lock: { mode: 'pessimistic_write' },
      });
      if (!order) throw new NotFoundException('订单不存在');
      if (order.status === OrderStatus.Paid) {
        order.status = OrderStatus.Refunding;
        await txMgr.save(PaymentOrder, order);
      }
      return order;
    });
  }

  async getOrderForRefund(orderNo: string, merchantId: number) {
    const order = await this.orderRepository.findOne({ where: { orderNo, merchantId } });
    if (!order) throw new NotFoundException('订单不存在');
    if (order.status !== OrderStatus.Paid) {
      throw new ConflictException(`订单状态不允许退款: ${order.status}`);
    }
    return order;
  }

  async markFailed(orderNo: string, reason?: string) {
    const order = await this.orderRepository.manager.transaction(async (txMgr) => {
      const order = await txMgr.findOne(PaymentOrder, {
        where: { orderNo },
        lock: { mode: 'pessimistic_write' },
      });
      if (!order) throw new NotFoundException('订单不存在');
      if (order.status !== OrderStatus.Pending) return order;

      order.status = OrderStatus.Failed;
      await txMgr.save(PaymentOrder, order);
      return order;
    });
    this.logger.log(`Order ${orderNo} marked as failed: ${reason}`);
    this.paymentGateway.notifyPaymentStatus(orderNo, 'failed');
    return order;
  }

  /**
   * Roll back a Refunding order to Paid when the upstream gateway rejects
   * or fails to process the refund. Prevents orders from getting permanently
   * stuck in the Refunding state.
   */
  async markRefundFailed(orderNo: string, reason?: string) {
    const order = await this.orderRepository.manager.transaction(async (txMgr) => {
      const order = await txMgr.findOne(PaymentOrder, {
        where: { orderNo },
        lock: { mode: 'pessimistic_write' },
      });
      if (!order) throw new NotFoundException('订单不存在');
      if (order.status !== OrderStatus.Refunding) return order;

      order.status = OrderStatus.Paid;
      await txMgr.save(PaymentOrder, order);
      return order;
    });
    this.logger.warn(`Order ${orderNo} refund failed, rolled back to Paid: ${reason}`);
    this.paymentGateway.notifyPaymentStatus(orderNo, 'refund_failed');
    return order;
  }

  private buildOrderResult(order: PaymentOrder) {
    return {
      orderNo: order.orderNo,
      amount: toYuan(order.amount),
      expireAt: order.expireAt,
    };
  }
}
