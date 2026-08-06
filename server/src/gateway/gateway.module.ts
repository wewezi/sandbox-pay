import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { Merchant } from '../entities/merchant.entity';
import { GatewayController } from './controllers/gateway.controller';
import { MerchantSignatureGuard } from './guards/merchant-signature.guard';
import { PaymentModule } from '../payment/payment.module';

@Module({
  imports: [
    TypeOrmModule.forFeature([Merchant]),
    PaymentModule,
  ],
  providers: [MerchantSignatureGuard],
  controllers: [GatewayController],
})
export class GatewayModule {}
