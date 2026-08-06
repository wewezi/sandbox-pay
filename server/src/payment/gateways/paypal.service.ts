import { Injectable, Logger, BadRequestException } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import * as paypal from '@paypal/checkout-server-sdk';
import { PaymentOrder } from '../../entities/payment-order.entity';
import { ExchangeRateService } from '../../common/services/exchange-rate.service';
import { PaymentService } from '../services/payment.service';
import { toYuan, toYuanString, yuanStringToCents } from '../../common/money';
import { errMessage, errStack } from '../../common/util/error';

/** Minimal shape of the PayPal SDK client we rely on. The SDK ships no types. */
interface PayPalHttpClient {
  execute<T = unknown>(request: unknown): Promise<{ result?: T; statusCode?: number }>;
}

interface PayPalLink { rel: string; href: string; method?: string }

interface PayPalOrderResult {
  id?: string;
  status?: string;
  links?: PayPalLink[];
  purchase_units?: Array<{
    payments?: {
      captures?: Array<{
        id?: string;
        amount?: { value?: string; currency_code?: string };
      }>;
    };
  }>;
}

interface PayPalRefundResult {
  id?: string;
  status?: string;
}

@Injectable()
export class PayPalService {
  private readonly logger = new Logger(PayPalService.name);
  private client: PayPalHttpClient;

  constructor(
    @InjectRepository(PaymentOrder)
    private orderRepository: Repository<PaymentOrder>,
    private configService: ConfigService,
    private exchangeRateService: ExchangeRateService,
    private paymentService: PaymentService,
  ) {
    const clientId = this.configService.get<string>('PAYPAL_CLIENT_ID')!;
    const clientSecret = this.configService.get<string>('PAYPAL_CLIENT_SECRET')!;
    const env = this.configService.get<string>('PAYPAL_ENVIRONMENT', 'sandbox');
    const environment =
      env === 'live'
        ? new paypal.core.LiveEnvironment(clientId, clientSecret)
        : new paypal.core.SandboxEnvironment(clientId, clientSecret);
    this.client = new paypal.core.PayPalHttpClient(environment);
    this.logger.log(`PayPal initialized in ${env} mode`);
  }

  async createOrder(orderNo: string, baseUrl: string) {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order) throw new BadRequestException('订单不存在');

    const rate = await this.exchangeRateService.getCnyToUsdRateAsync();
    // order.amount is CNY cents; convert to USD cents via the rate.
    const usdCents = Math.round(toYuan(order.amount) * rate * 100);

    order.exchangeRate = rate;
    order.foreignAmount = usdCents;
    order.foreignCurrency = 'USD';
    await this.orderRepository.save(order);

    const request = new paypal.orders.OrdersCreateRequest();
    request.prefer('return=representation');
    request.requestBody({
      intent: 'CAPTURE',
      purchase_units: [{
        amount: {
          currency_code: 'USD',
          value: toYuanString(usdCents),
        },
        description: order.productName,
        reference_id: order.orderNo,
      }],
      application_context: {
        return_url: `${baseUrl}/api/paypal/callback?orderNo=${order.orderNo}`,
        cancel_url: `${baseUrl}/api/paypal/callback?orderNo=${order.orderNo}&cancel=1`,
      },
    });

    try {
      const response = await this.client.execute<PayPalOrderResult>(request);
      const approveUrl = response.result?.links?.find(link => link.rel === 'approve')?.href;
      if (!approveUrl) {
        throw new Error('PayPal 响应缺少确认链接');
      }
      return { type: 'url', data: approveUrl };
    } catch (err: unknown) {
      this.logger.error(`PayPal order creation failed: ${errMessage(err)}`, errStack(err));
      throw new BadRequestException('PayPal 下单失败');
    }
  }

  /**
   * Refund a PayPal capture. `captureId` is the `thirdPartyTradeNo` stored on the order.
   * @param refundCents integer minor units of `currency` to refund.
   */
  async refund(captureId: string, refundCents: number, currency: string = 'USD', reason?: string): Promise<string | null> {
    try {
      const request = new paypal.payments.CapturesRefundRequest(captureId);
      request.requestBody({
        amount: { value: toYuanString(refundCents), currency_code: currency },
        note_to_payer: reason || 'Merchant initiated refund',
      } as any);
      const response = await this.client.execute<PayPalRefundResult>(request);
      const refundId = response?.result?.id;
      const status = response?.result?.status;
      if (refundId && (status === 'COMPLETED' || status === 'PENDING')) {
        this.logger.log(`PayPal refund initiated for capture ${captureId}: ${refundId} (${status})`);
        return refundId;
      }
      this.logger.warn(`PayPal refund unexpected response: ${JSON.stringify(response?.result)}`);
      return null;
    } catch (err: unknown) {
      this.logger.error(`PayPal refund failed for capture ${captureId}: ${errMessage(err)}`);
      return null;
    }
  }

  async captureOrder(orderNo: string, token: string) {
    const request = new paypal.orders.OrdersCaptureRequest(token);
    request.requestBody({} as any);

    try {
      const response = await this.client.execute<PayPalOrderResult>(request);
      if (response.result?.status === 'COMPLETED') {
        const capture = response.result.purchase_units?.[0]?.payments?.captures?.[0];
        const captureId = capture?.id;
        // PayPal returns `value` as a yuan-like decimal string (e.g. "10.00");
        // convert to integer minor units at the boundary.
        const capturedCents = yuanStringToCents(capture?.amount?.value);
        const capturedCurrency = String(capture?.amount?.currency_code || 'USD').toUpperCase();

        if (!captureId || !Number.isFinite(capturedCents) || capturedCents <= 0) {
          this.logger.error(`PayPal capture missing id/amount for ${orderNo}`);
          await this.paymentService.markFailed(orderNo, 'PayPal capture missing fields');
          return { success: false };
        }

        try {
          await this.paymentService.markPaid(orderNo, captureId, capturedCents, capturedCurrency);
          this.logger.log(
            `PayPal order ${orderNo} captured: ${captureId} (${capturedCents} ${capturedCurrency} cents)`,
          );
          return { success: true, captureId };
        } catch (err: unknown) {
          // Amount mismatch — do NOT mark failed (capture is real money received); flag for reconciliation.
          this.logger.error(
            `PayPal capture amount validation rejected for ${orderNo}: ${errMessage(err)}. ` +
              `Capture ${captureId} requires manual reconciliation.`,
          );
          return { success: false, captureId, error: 'amount_mismatch' };
        }
      }
      await this.paymentService.markFailed(orderNo, 'PayPal capture not completed');
      return { success: false };
    } catch (err: unknown) {
      const message = errMessage(err);
      this.logger.error(`PayPal capture failed for ${orderNo}: ${message}`, errStack(err));
      await this.paymentService.markFailed(orderNo, message);
      return { success: false, error: message };
    }
  }
}
