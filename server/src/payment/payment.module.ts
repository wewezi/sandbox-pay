import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { PaymentOrder } from '../entities/payment-order.entity';
import { Merchant } from '../entities/merchant.entity';
import { PaymentService } from './services/payment.service';
import { RefundService } from './services/refund.service';
import { AlipayService } from './gateways/alipay.service';
import { PayPalService } from './gateways/paypal.service';
import { NativePayService } from './gateways/native-pay.service';
import { PaymentGateway } from './payment.gateway';
import { AlipayController } from './controllers/alipay.controller';
import { PayPalController } from './controllers/paypal.controller';
import { NativePayController } from './controllers/native-pay.controller';

@Module({
  imports: [
    TypeOrmModule.forFeature([PaymentOrder, Merchant]),
  ],
  providers: [PaymentService, RefundService, AlipayService, PayPalService, NativePayService, PaymentGateway],
  controllers: [AlipayController, PayPalController, NativePayController],
  exports: [PaymentService, RefundService, AlipayService, PayPalService, NativePayService, PaymentGateway],
})
export class PaymentModule {}
