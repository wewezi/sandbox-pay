import { Controller, Get, Query, Response, Logger } from '@nestjs/common';
import type { Response as ExpressResponse } from 'express';
import { ConfigService } from '@nestjs/config';
import { Throttle } from '@nestjs/throttler';
import { PaymentService } from '../services/payment.service';
import { PayPalService } from '../gateways/paypal.service';
import { OrderStatus } from '../../entities/payment-order.entity';
import { ApiTags, ApiOperation } from '@nestjs/swagger';

@ApiTags('PayPal')
@Controller('api/paypal')
export class PayPalController {
  private readonly logger = new Logger(PayPalController.name);

  constructor(
    private paymentService: PaymentService,
    private paypalService: PayPalService,
    private configService: ConfigService,
  ) {}

  @Get('callback')
  @Throttle({ default: { limit: 10, ttl: 60_000 } })
  @ApiOperation({ summary: 'PayPal return callback - capture and redirect' })
  async callback(
    @Query('orderNo') orderNo: string,
    @Query('token') token: string,
    @Query('cancel') cancel: string,
    @Response() res: ExpressResponse,
  ) {
    if (!orderNo) return res.redirect('/?error=missing_order_no');

    // 通过 PaymentService 查询订单，不再直接访问 repo
    let order: Awaited<ReturnType<typeof this.paymentService.queryOrder>>;
    try {
      order = await this.paymentService.queryOrder(orderNo);
    } catch {
      return res.redirect('/?error=order_not_found');
    }

    const clientUrl = this.configService.get<string>('CLIENT_URL') || '';
    const defaultReturnUrl = clientUrl ? `${clientUrl}/cashier` : '/cashier';
    const returnUrl = order.returnUrl || defaultReturnUrl;

    if (cancel) {
      return res.redirect(this.buildReturnUrl(returnUrl, orderNo, 'cancelled'));
    }

    if (order.status !== OrderStatus.Pending) {
      this.logger.warn(`PayPal callback for non-pending order ${orderNo} (status: ${order.status}), skipping capture`);
      return res.redirect(this.buildReturnUrl(returnUrl, orderNo, order.status === OrderStatus.Paid ? 'success' : 'failed'));
    }

    if (token) {
      try {
        const result = await this.paypalService.captureOrder(orderNo, token);
        if (result.success) {
          return res.redirect(this.buildReturnUrl(returnUrl, orderNo, 'success'));
        }
      } catch (err: unknown) {
        this.logger.error(`PayPal capture failed for ${orderNo}: ${err instanceof Error ? err.message : String(err)}`);
      }
    }

    return res.redirect(this.buildReturnUrl(returnUrl, orderNo, 'failed'));
  }

  private buildReturnUrl(returnUrl: string, orderNo: string, status: string): string {
    const separator = returnUrl.includes('?') ? '&' : '?';
    return `${returnUrl}${separator}orderNo=${orderNo}&status=${status}`;
  }
}
