import { Controller, Post, Get, Body, UseGuards, Request, Query, BadRequestException } from '@nestjs/common';
import { MerchantSignatureGuard } from '../guards/merchant-signature.guard';
import { PaymentService } from '../../payment/services/payment.service';
import { RefundService } from '../../payment/services/refund.service';
import { PayPalService } from '../../payment/gateways/paypal.service';
import { AlipayService } from '../../payment/gateways/alipay.service';
import { NativePayService } from '../../payment/gateways/native-pay.service';
import { CreatePaymentDto, RefundDto } from '../../payment/dto/payment.dto';
import { ApiTags, ApiOperation, ApiHeader } from '@nestjs/swagger';
import type { AuthenticatedRequest } from '../../common/types/express';
import { clientIp } from '../../common/util/request';

@ApiTags('Gateway')
@Controller('api/gateway')
export class GatewayController {
  constructor(
    private paymentService: PaymentService,
    private refundService: RefundService,
    private paypalService: PayPalService,
    private alipayService: AlipayService,
    private nativePayService: NativePayService,
  ) {}

  @Post('pay')
  @UseGuards(MerchantSignatureGuard)
  @ApiOperation({ summary: 'Create a payment order and get payment URL' })
  @ApiHeader({ name: 'X-Sandbox-Pay-AppKey', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Timestamp', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Nonce', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Signature', required: true })
  async createPayment(@Request() req: AuthenticatedRequest, @Body() dto: CreatePaymentDto) {
    const merchant = req.merchant!;
    const order = await this.paymentService.createOrder(merchant.id, dto);
    const baseUrl = `${req.protocol}://${req.get('host')}`;

    switch (dto.payMethod) {
      case 'paypal':
        return this.paypalService.createOrder(order.orderNo, baseUrl);
      case 'alipay':
        return this.alipayService.createPagePay(order.orderNo, baseUrl);
      case 'native':
        return this.nativePayService.createOrder(order.orderNo, baseUrl);
      default:
        throw new BadRequestException(`Unsupported payment method: ${dto.payMethod}`);
    }
  }

  // ── 渠道专属端点（无需传 payMethod） ────────────────────────

  @Post('alipay/pay')
  @UseGuards(MerchantSignatureGuard)
  @ApiOperation({ summary: 'Direct: create Alipay payment order' })
  @ApiHeader({ name: 'X-Sandbox-Pay-AppKey', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Timestamp', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Nonce', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Signature', required: true })
  async alipayPay(@Request() req: AuthenticatedRequest, @Body() dto: Omit<CreatePaymentDto, 'payMethod'>) {
    const merchant = req.merchant!;
    const order = await this.paymentService.createOrder(merchant.id, { ...dto, payMethod: 'alipay' });
    const baseUrl = `${req.protocol}://${req.get('host')}`;
    return this.alipayService.createPagePay(order.orderNo, baseUrl);
  }

  @Post('paypal/pay')
  @UseGuards(MerchantSignatureGuard)
  @ApiOperation({ summary: 'Direct: create PayPal payment order' })
  @ApiHeader({ name: 'X-Sandbox-Pay-AppKey', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Timestamp', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Nonce', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Signature', required: true })
  async paypalPay(@Request() req: AuthenticatedRequest, @Body() dto: Omit<CreatePaymentDto, 'payMethod'>) {
    const merchant = req.merchant!;
    const order = await this.paymentService.createOrder(merchant.id, { ...dto, payMethod: 'paypal' });
    const baseUrl = `${req.protocol}://${req.get('host')}`;
    return this.paypalService.createOrder(order.orderNo, baseUrl);
  }

  @Post('native/pay')
  @UseGuards(MerchantSignatureGuard)
  @ApiOperation({ summary: 'Direct: create Native (escrow) payment order' })
  @ApiHeader({ name: 'X-Sandbox-Pay-AppKey', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Timestamp', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Nonce', required: true })
  @ApiHeader({ name: 'X-Sandbox-Pay-Signature', required: true })
  async nativePay(@Request() req: AuthenticatedRequest, @Body() dto: Omit<CreatePaymentDto, 'payMethod'>) {
    const merchant = req.merchant!;
    const order = await this.paymentService.createOrder(merchant.id, { ...dto, payMethod: 'native' });
    const baseUrl = `${req.protocol}://${req.get('host')}`;
    return this.nativePayService.createOrder(order.orderNo, baseUrl);
  }

  // ── 通用查询与退款 ──────────────────────────────────────────

  @Get('query')
  @UseGuards(MerchantSignatureGuard)
  @ApiOperation({ summary: 'Query order status by orderNo' })
  async queryOrder(@Request() req: AuthenticatedRequest, @Query('orderNo') orderNo: string) {
    const merchant = req.merchant!;
    return this.paymentService.queryByMerchant(merchant.id, orderNo);
  }

  @Post('refund')
  @UseGuards(MerchantSignatureGuard)
  @ApiOperation({ summary: 'Refund a paid order (full or partial)' })
  async refund(@Request() req: AuthenticatedRequest, @Body() dto: RefundDto) {
    const merchant = req.merchant!;
    return this.refundService.execute(dto, {
      merchantId: merchant.id,
      actor: `merchant:${merchant.id}`,
      ip: clientIp(req),
    });
  }
}
