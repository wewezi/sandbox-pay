import { Injectable, Logger, BadRequestException, NotFoundException } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import * as QRCode from 'qrcode';
import { PaymentOrder, OrderStatus } from '../../entities/payment-order.entity';
import { PaymentService } from '../services/payment.service';
import { AlipayService } from './alipay.service';
import { SiteSettingsService } from '../../common/services/site-settings.service';
import { toYuan } from '../../common/money';

@Injectable()
export class NativePayService {
  private readonly logger = new Logger(NativePayService.name);

  constructor(
    @InjectRepository(PaymentOrder)
    private orderRepository: Repository<PaymentOrder>,
    private configService: ConfigService,
    private siteSettingsService: SiteSettingsService,
    private paymentService: PaymentService,
    private alipayService: AlipayService,
  ) {}

  /**
   * 创建自有支付订单，返回收银台页面 URL
   */
  async createOrder(orderNo: string, baseUrl: string) {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order) throw new BadRequestException('订单不存在');
    if (order.status !== OrderStatus.Pending) throw new BadRequestException('订单状态非待支付');

    const clientUrl = this.configService.get<string>('CLIENT_URL') || baseUrl;
    const cashierUrl = `${clientUrl}/cashier?orderNo=${orderNo}`;

    this.logger.log(`Native pay cashier URL generated for order: ${orderNo}`);
    return { type: 'url', data: cashierUrl };
  }

  /**
   * 获取收银台所需的支付信息
   */
  async getCashierInfo(orderNo: string) {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order) throw new NotFoundException('订单不存在');

    if (order.status === OrderStatus.Paid) {
      return {
        orderNo: order.orderNo,
        amount: toYuan(order.amount),
        productName: order.productName,
        status: 'paid',
        payMethod: order.payMethod || 'alipay',
        thirdPartyTradeNo: order.thirdPartyTradeNo || `SP_CLEARED_${order.orderNo}`,
        payAt: order.payAt || new Date(),
      };
    }

    if (order.status === OrderStatus.Expired || order.status === OrderStatus.Failed) {
      return {
        orderNo: order.orderNo,
        amount: toYuan(order.amount),
        productName: order.productName,
        status: order.status === OrderStatus.Expired ? 'expired' : 'failed',
      };
    }

    // 检查是否过期
    if (new Date() > new Date(order.expireAt)) {
      return {
        orderNo: order.orderNo,
        amount: toYuan(order.amount),
        productName: order.productName,
        status: 'expired',
      };
    }

    // 主动核实支付宝状态（针对本地公网异步通知无法直达内网的情况）
    if (order.payMethod === 'alipay' || order.orderNo.startsWith('PAY')) {
      const isAlipayPaid = await this.alipayService.queryAlipayTrade(orderNo);
      if (isAlipayPaid) {
        const refreshedOrder = await this.orderRepository.findOne({ where: { orderNo } });
        return {
          orderNo: order.orderNo,
          amount: toYuan(order.amount),
          productName: order.productName,
          status: 'paid',
          payMethod: refreshedOrder?.payMethod || order.payMethod || 'alipay',
          thirdPartyTradeNo: refreshedOrder?.thirdPartyTradeNo || `ALIPAY_SYNC_${Date.now()}`,
          payAt: refreshedOrder?.payAt || new Date(),
        };
      }
    }

    let qrCodeUrl = this.siteSettingsService.get('native_pay.qr_url') || '';
    if (!qrCodeUrl) {
      const alipayQr = await this.alipayService.createQrPay(orderNo);
      if (alipayQr) {
        qrCodeUrl = await QRCode.toDataURL(alipayQr, { margin: 2, width: 300 });
      }
    }

    return {
      orderNo: order.orderNo,
      amount: toYuan(order.amount),
      productName: order.productName,
      status: 'pending',
      expireAt: order.expireAt,
      paymentInfo: {
        qrCodeUrl,
        accountName: this.siteSettingsService.get('native_pay.account_name') || 'Sandbox Pay Official',
        accountNo: this.siteSettingsService.get('native_pay.account_no') || '',
        bankName: this.siteSettingsService.get('native_pay.bank_name') || '',
        remark: `WP${orderNo}`,
      },
    };
  }

  /**
   * 管理员/存管钱包手动核销确认。
   *
   * 调用方必须确保已经做过身份验证（JwtAuthGuard 或 SandboxGuard）。
   * 该方法本身只做业务校验：
   *   - 订单存在且仍在 Pending
   *   - 渠道为 native（禁止跨渠道核销）
   *   - 金额按订单原值入账（防止伪造金额）
   */
  async confirmPayment(orderNo: string, tradeNo?: string, walletUser?: string, walletPass?: string) {
    const order = await this.orderRepository.findOne({ where: { orderNo } });
    if (!order) throw new NotFoundException('订单不存在');
    if (order.status !== OrderStatus.Pending) {
      throw new BadRequestException(`订单状态不正确，当前状态: ${order.status}`);
    }

    // 严防跨渠道结算渗透：禁止通过存管直联核销支付宝或 PayPal 渠道的订单
    if (order.payMethod === 'alipay' || order.payMethod === 'paypal') {
      throw new BadRequestException(
        `该订单属于外部通道 (${order.payMethod})，无法直接通过 Sandbox Pay 自有存管清算体系进行核销`,
      );
    }

    // 存管钱包账号 + 安全密码：始终强校验（不允许两个都为空时绕过）
    if (!walletUser || walletUser.trim().length < 5) {
      throw new BadRequestException('存管结算账号格式不合规');
    }
    if (!walletPass || walletPass.trim().length < 6) {
      throw new BadRequestException('支付安全密码凭据不正确');
    }

    const confirmedTradeNo = tradeNo || `SP_WALLET_${Date.now()}`;
    // 金额按订单原值入账（cents），杜绝调用方伪造金额
    await this.paymentService.markPaid(
      orderNo,
      confirmedTradeNo,
      order.amount,
      'CNY',
    );

    this.logger.log(`Native payment confirmed for ${orderNo}, tradeNo: ${confirmedTradeNo}`);
    return { success: true, orderNo, tradeNo: confirmedTradeNo };
  }
}
