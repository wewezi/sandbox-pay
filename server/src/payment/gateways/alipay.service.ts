import { Injectable, Logger, BadRequestException, InternalServerErrorException } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { AlipaySdk } from 'alipay-sdk';
import { PaymentOrder, OrderStatus } from '../../entities/payment-order.entity';
import { PaymentService } from '../services/payment.service';
import { toYuanString, yuanStringToCents } from '../../common/money';
import { errMessage, errStack } from '../../common/util/error';

/**
 * Loose shape for the JSON body returned by Alipay's OpenAPI. The SDK types it
 * as `any`; we wrap reads through this index signature so reads are explicit
 * `unknown` rather than implicit `any`.
 */
type AlipayResult = Record<string, unknown> & {
  alipayTradePrecreateResponse?: Record<string, unknown>;
  alipayTradeRefundResponse?: Record<string, unknown>;
  alipayTradeQueryResponse?: Record<string, unknown>;
};

function str(v: unknown): string | undefined {
  return typeof v === 'string' ? v : undefined;
}

@Injectable()
export class AlipayService {
  private readonly logger = new Logger(AlipayService.name);
  private readonly alipaySdk: AlipaySdk;

  constructor(
    @InjectRepository(PaymentOrder)
    private orderRepository: Repository<PaymentOrder>,
    private configService: ConfigService,
    private paymentService: PaymentService,
  ) {
    this.alipaySdk = new AlipaySdk({
      appId: this.configService.get<string>('ALIPAY_APP_ID')!,
      privateKey: this.configService.get<string>('ALIPAY_PRIVATE_KEY')!,
      alipayPublicKey: this.configService.get<string>('ALIPAY_PUBLIC_KEY')!,
      gateway: this.configService.get<string>('ALIPAY_SERVER_URL')!,
    });
  }

  async createPagePay(orderNo: string, baseUrl: string) {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order) throw new BadRequestException('订单不存在');
    if (order.status !== OrderStatus.Pending) throw new BadRequestException('订单状态非待支付');

    const notifyUrl = `${baseUrl}/api/alipay/notify`;
    const clientUrl = this.configService.get<string>('CLIENT_URL') || baseUrl;
    const defaultReturnUrl = `${clientUrl}/cashier`;
    const returnUrl = order.returnUrl
      ? `${order.returnUrl}${order.returnUrl.includes('?') ? '&' : '?'}orderNo=${orderNo}`
      : `${defaultReturnUrl}?orderNo=${orderNo}`;

    try {
      const result = this.alipaySdk.pageExec('alipay.trade.page.pay', {
        notify_url: notifyUrl,
        return_url: returnUrl,
        biz_content: {
          out_trade_no: order.orderNo,
          product_code: 'FAST_INSTANT_TRADE_PAY',
          total_amount: toYuanString(order.amount),
          subject: order.productName,
        },
      });

      this.logger.log(`Alipay page pay form generated for order: ${orderNo}`);
      return { type: 'form', data: result };
    } catch (err: unknown) {
      this.logger.error(`Alipay page pay error: ${errMessage(err)}`, errStack(err));
      throw new InternalServerErrorException('生成支付宝支付表单失败');
    }
  }

  async handleNotify(params: Record<string, string>) {
    const isValid = await this.alipaySdk.checkNotifySign(params);
    if (!isValid) {
      this.logger.warn('Alipay notify signature verification failed');
      return 'failure';
    }

    const { out_trade_no, trade_no, trade_status, total_amount } = params;
    if (trade_status === 'TRADE_SUCCESS' || trade_status === 'TRADE_FINISHED') {
      try {
        // Alipay returns yuan strings like "10.00"; convert at the boundary to cents.
        await this.paymentService.markPaid(
          out_trade_no,
          trade_no,
          yuanStringToCents(total_amount),
          'CNY',
        );
        this.logger.log(`Alipay payment success: ${out_trade_no}, trade_no: ${trade_no}`);
      } catch (err: unknown) {
        // Amount mismatch or other validation failure — record and tell Alipay to retry later.
        this.logger.error(
          `Alipay notify rejected for ${out_trade_no}: ${errMessage(err)}`,
        );
        return 'failure';
      }
    }

    return 'success';
  }

  async createQrPay(orderNo: string): Promise<string | null> {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order || order.status !== OrderStatus.Pending) return null;

    const baseUrl = this.configService.get<string>('CLIENT_URL') || 'http://localhost:3000';
    const notifyUrl = `${baseUrl}/api/alipay/notify`;

    try {
      const result = (await this.alipaySdk.exec('alipay.trade.precreate', {
        notify_url: notifyUrl,
        bizContent: {
          out_trade_no: order.orderNo,
          total_amount: toYuanString(order.amount),
          subject: order.productName,
        },
      })) as AlipayResult;

      this.logger.log(`Alipay precreate result for ${orderNo}: ${JSON.stringify(result)}`);
      const response = (result.alipayTradePrecreateResponse ?? result) as Record<string, unknown>;
      const subCode = str(response.subCode) ?? str(response.sub_code);
      if (subCode === 'ACQ.TRADE_HAS_SUCCESS') {
        // Already paid in Alipay — actively query to get the true trade_no & amount,
        // then let queryAlipayTrade handle markPaid with proper amount validation.
        this.logger.log(`Alipay precreate detected order ${orderNo} already paid, switching to active query.`);
        await this.queryAlipayTrade(orderNo);
        return null;
      }
      return str(response.qrCode) ?? str(response.qr_code) ?? null;
    } catch (err: unknown) {
      this.logger.error(`Alipay precreate error: ${errMessage(err)}`, errStack(err));
      return null;
    }
  }

  /**
   * Initiate a refund through Alipay. Uses Alipay's `out_request_no` for idempotency.
   * @param refundCents integer cents to refund
   */
  async refund(orderNo: string, refundCents: number, reason?: string): Promise<string | null> {
    try {
      const outRequestNo = `RF_${orderNo}_${Date.now()}`;
      const result = (await this.alipaySdk.exec('alipay.trade.refund', {
        bizContent: {
          out_trade_no: orderNo,
          refund_amount: toYuanString(refundCents),
          refund_reason: reason || 'Merchant initiated refund',
          out_request_no: outRequestNo,
        },
      })) as AlipayResult;
      const response = (result.alipayTradeRefundResponse ?? result) as Record<string, unknown>;
      const fundChangeOk = response.fundChange === 'Y' || response.fund_change === 'Y';
      if ((response.code === '10000' || response.msg === 'Success') && fundChangeOk) {
        this.logger.log(`Alipay refund success for ${orderNo}: ${outRequestNo}`);
        return outRequestNo;
      }
      this.logger.warn(
        `Alipay refund non-success for ${orderNo}: ${JSON.stringify(response)}`,
      );
      return null;
    } catch (err: unknown) {
      this.logger.error(`Alipay refund error for ${orderNo}: ${errMessage(err)}`);
      return null;
    }
  }

  async queryAlipayTrade(orderNo: string): Promise<boolean> {
    try {
      const result = (await this.alipaySdk.exec('alipay.trade.query', {
        bizContent: { out_trade_no: orderNo },
      })) as AlipayResult;
      const response = (result.alipayTradeQueryResponse ?? result) as Record<string, unknown>;
      const status = str(response.tradeStatus) ?? str(response.trade_status);
      if (status === 'TRADE_SUCCESS' || status === 'TRADE_FINISHED') {
        const tradeNo = str(response.tradeNo) ?? str(response.trade_no);
        const totalAmountRaw = str(response.totalAmount) ?? str(response.total_amount) ?? '0';
        const totalAmountCents = yuanStringToCents(totalAmountRaw);
        if (!tradeNo || !Number.isFinite(totalAmountCents) || totalAmountCents <= 0) {
          this.logger.warn(`Alipay query missing tradeNo/totalAmount for ${orderNo}, skip markPaid`);
          return false;
        }
        try {
          await this.paymentService.markPaid(orderNo, tradeNo, totalAmountCents, 'CNY');
          this.logger.log(`Alipay active query confirmed paid for ${orderNo}, tradeNo: ${tradeNo}`);
          return true;
        } catch (err: unknown) {
          this.logger.error(`Alipay active query markPaid rejected for ${orderNo}: ${errMessage(err)}`);
          return false;
        }
      }
      return false;
    } catch (err: unknown) {
      this.logger.error(`Alipay queryAlipayTrade error for ${orderNo}: ${errMessage(err)}`);
      return false;
    }
  }
}
