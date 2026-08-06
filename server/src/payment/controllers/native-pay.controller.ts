import { Controller, Get, Post, Query, Body, UseGuards, Req, BadRequestException, Logger } from '@nestjs/common';
import type { Request } from 'express';
import { Throttle } from '@nestjs/throttler';
import { NativePayService } from '../gateways/native-pay.service';
import { AlipayService } from '../gateways/alipay.service';
import { PayPalService } from '../gateways/paypal.service';
import { PaymentService } from '../services/payment.service';
import { MerchantService } from '../../common/services/merchant.service';
import { ConfirmPaymentDto, SandboxConfirmDto, SwitchChannelDto } from '../dto/payment.dto';
import { ApiTags, ApiOperation } from '@nestjs/swagger';
import { JwtAuthGuard } from '../../auth/jwt-auth.guard';
import { SandboxGuard } from '../../common/guards/sandbox.guard';
import { errMessage, errStack } from '../../common/util/error';

@ApiTags('NativePay')
@Controller('api/native-pay')
export class NativePayController {
  private readonly logger = new Logger(NativePayController.name);

  constructor(
    private nativePayService: NativePayService,
    private alipayService: AlipayService,
    private paypalService: PayPalService,
    private paymentService: PaymentService,
    private merchantService: MerchantService,
  ) {}

  @Get('cashier')
  @ApiOperation({ summary: 'Get cashier page payment info for native pay' })
  async getCashierInfo(@Query('orderNo') orderNo: string) {
    return this.nativePayService.getCashierInfo(orderNo);
  }

  /**
   * Public sandbox test endpoint — creates a test order without authentication.
   * Only available when ENABLE_SANDBOX=true.
   */
  @Post('public-test-pay')
  @UseGuards(SandboxGuard)
  @Throttle({ default: { limit: 10, ttl: 60_000 } })
  @ApiOperation({ summary: '[SANDBOX] Create a public test order (no auth required)' })
  async publicTestPay(@Req() req: Request) {
    const merchant = await this.merchantService.findActiveMerchant();
    const orderResult = await this.paymentService.createOrder(merchant.id, {
      amount: 0.01,
      productName: 'Sandbox Pay 公开测试订单',
      payMethod: 'native',
    });
    const baseUrl = `${req.protocol}://${req.get('host')}`;
    return this.nativePayService.createOrder(orderResult.orderNo, baseUrl);
  }

  @Post('confirm')
  @UseGuards(JwtAuthGuard)
  @ApiOperation({ summary: 'Admin manually confirm payment received' })
  async confirmPayment(@Body() dto: ConfirmPaymentDto) {
    return this.nativePayService.confirmPayment(
      dto.orderNo,
      dto.tradeNo,
      dto.walletUser,
      dto.walletPass,
    );
  }

  @Post('sandbox-confirm')
  @UseGuards(SandboxGuard)
  @ApiOperation({ summary: '[SANDBOX-ONLY] confirm payment for local testing (disabled in production)' })
  async sandboxConfirm(@Body() dto: SandboxConfirmDto) {
    return this.nativePayService.confirmPayment(
      dto.orderNo,
      'SP_WALLET_' + Date.now(),
      dto.walletUser,
      dto.walletPass,
    );
  }

  @Post('switch-channel')
  @Throttle({ default: { limit: 5, ttl: 60_000 } }) // 5 requests per minute — prevents gateway abuse
  @ApiOperation({ summary: 'Directly invoke real payment gateways (Alipay / PayPal) from cashier' })
  async switchChannel(@Body() dto: SwitchChannelDto, @Req() req: Request) {
    const baseUrl = `${req.protocol}://${req.get('host')}`;
    try {
      if (dto.channel === 'alipay') {
        return await this.alipayService.createPagePay(dto.orderNo, baseUrl);
      } else if (dto.channel === 'paypal') {
        return await this.paypalService.createOrder(dto.orderNo, baseUrl);
      }
      throw new BadRequestException('不支持的支付渠道');
    } catch (err: unknown) {
      const message = errMessage(err);
      this.logger.error(`Switch channel error: ${message}`, errStack(err));
      throw new BadRequestException(message || '网关调用失败');
    }
  }

}

