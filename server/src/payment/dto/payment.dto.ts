import { IsString, IsNumber, IsOptional, Min, IsUrl, MaxLength, IsIn } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';

export class RefundDto {
  @ApiProperty({ description: 'Original Sandbox Pay orderNo' })
  @IsString()
  orderNo: string;

  @ApiProperty({ example: 10, description: 'Refund amount (CNY for native/alipay, foreign currency for paypal)' })
  @IsNumber()
  @Min(0.01)
  amount: number;

  @ApiProperty({ required: false })
  @IsOptional()
  @IsString()
  @MaxLength(255)
  reason?: string;
}

export class CreatePaymentDto {
  @ApiProperty({ example: 100 })
  @IsNumber()
  @Min(0.01)
  amount: number;

  @ApiProperty({ example: 'Test Product' })
  @IsString()
  @MaxLength(200)
  productName: string;

  @ApiProperty({ example: 'alipay', description: 'Payment method: alipay, paypal, native' })
  @IsString()
  @IsIn(['alipay', 'paypal', 'native'])
  payMethod: string;

  @ApiProperty({ required: false })
  @IsOptional()
  @IsString()
  externalOrderNo?: string;

  @ApiProperty({ required: false })
  @IsOptional()
  @IsUrl()
  returnUrl?: string;

  @ApiProperty({ required: false })
  @IsOptional()
  @IsUrl()
  notifyUrl?: string;
}

export class TestPayDto {
  @ApiProperty({ example: 88.88, description: 'Test order amount in yuan' })
  @IsNumber()
  @Min(0.01)
  amount: number;

  @ApiProperty({ example: 'Sandbox Pay 测试订单', required: false })
  @IsOptional()
  @IsString()
  @MaxLength(200)
  productName?: string;
}

export class UploadBillDto {
  @ApiProperty({ example: 'alipay', enum: ['alipay', 'paypal'] })
  @IsString()
  @IsIn(['alipay', 'paypal'])
  provider: 'alipay' | 'paypal';

  @ApiProperty({ example: '2025-06-22', description: 'Bill date in YYYY-MM-DD format' })
  @IsString()
  billDate: string;

  @ApiProperty({ description: 'Raw CSV content from the upstream PSP' })
  @IsString()
  csv: string;
}

export class SandboxConfirmDto {
  @ApiProperty({ description: 'Order number to confirm' })
  @IsString()
  orderNo: string;

  @ApiProperty({ required: false, description: 'Sandbox wallet username' })
  @IsOptional()
  @IsString()
  walletUser?: string;

  @ApiProperty({ required: false, description: 'Sandbox wallet password' })
  @IsOptional()
  @IsString()
  walletPass?: string;
}

export class ConfirmPaymentDto {
  @ApiProperty({ description: 'Order number to confirm' })
  @IsString()
  orderNo: string;

  @ApiProperty({ required: false, description: 'Third-party trade number' })
  @IsOptional()
  @IsString()
  tradeNo?: string;

  @ApiProperty({ description: 'Wallet username' })
  @IsString()
  walletUser: string;

  @ApiProperty({ description: 'Wallet password' })
  @IsString()
  walletPass: string;
}

export class SwitchChannelDto {
  @ApiProperty({ description: 'Order number' })
  @IsString()
  orderNo: string;

  @ApiProperty({ enum: ['alipay', 'paypal'], description: 'Target payment channel' })
  @IsString()
  @IsIn(['alipay', 'paypal'])
  channel: 'alipay' | 'paypal';
}
