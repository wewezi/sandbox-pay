import { Controller, Get, Post, Patch, Delete, Param, Body, Query, UseGuards, Req, BadRequestException } from '@nestjs/common';
import { AdminService } from './admin.service';
import { ReconciliationService } from './reconciliation.service';
import { ReconStatus } from '../entities/reconciliation-record.entity';
import { RefundService } from '../payment/services/refund.service';
import { PaymentService } from '../payment/services/payment.service';
import { NativePayService } from '../payment/gateways/native-pay.service';
import { AuditService } from '../common/services/audit.service';
import { SiteSettingsService } from '../common/services/site-settings.service';
import { MerchantService } from '../common/services/merchant.service';
import { SandboxGuard } from '../common/guards/sandbox.guard';
import { RefundDto, TestPayDto, UploadBillDto } from '../payment/dto/payment.dto';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { ApiTags, ApiOperation, ApiBearerAuth, ApiQuery } from '@nestjs/swagger';
import type { AuthenticatedRequest } from '../common/types/express';
import { clientIp } from '../common/util/request';

@ApiTags('Admin')
@ApiBearerAuth()
@UseGuards(JwtAuthGuard)
@Controller('api/admin')
export class AdminController {
  constructor(
    private readonly adminService: AdminService,
    private readonly reconciliationService: ReconciliationService,
    private readonly refundService: RefundService,
    private readonly paymentService: PaymentService,
    private readonly nativePayService: NativePayService,
    private readonly auditService: AuditService,
    private readonly siteSettingsService: SiteSettingsService,
    private readonly merchantService: MerchantService,
  ) {}

  /** Resolve a stable actor identifier from the JWT payload attached by `JwtAuthGuard`. */
  private actorOf(req: AuthenticatedRequest): string {
    const u = req?.user?.username || req?.user?.sub || 'admin';
    return `admin:${u}`;
  }


  @Get('stats')
  @ApiOperation({ summary: 'Get total amount, success count and routing stats' })
  @ApiQuery({ name: 'merchantId', required: false, type: Number })
  async getStats(@Query('merchantId') merchantId?: string) {
    return this.adminService.getStats(merchantId ? Number(merchantId) : undefined);
  }

  @Get('transactions')
  @ApiOperation({ summary: 'Get paginated payment orders with filters' })
  @ApiQuery({ name: 'merchantId', required: false, type: Number })
  @ApiQuery({ name: 'page', required: false, type: Number })
  @ApiQuery({ name: 'pageSize', required: false, type: Number })
  @ApiQuery({ name: 'status', required: false, description: 'Order status: 0=Pending, 1=Paid, 2=Failed, 3=Refunding, 4=Refunded, -1=Expired' })
  @ApiQuery({ name: 'payMethod', required: false, description: 'Payment method: alipay, paypal, native' })
  @ApiQuery({ name: 'dateFrom', required: false, description: 'Start date (YYYY-MM-DD)' })
  @ApiQuery({ name: 'dateTo', required: false, description: 'End date (YYYY-MM-DD)' })
  @ApiQuery({ name: 'keyword', required: false, description: 'Search by orderNo or externalOrderNo' })
  async getTransactions(
    @Query('merchantId') merchantId?: string,
    @Query('page') page?: string,
    @Query('pageSize') pageSize?: string,
    @Query('status') status?: string,
    @Query('payMethod') payMethod?: string,
    @Query('dateFrom') dateFrom?: string,
    @Query('dateTo') dateTo?: string,
    @Query('keyword') keyword?: string,
  ) {
    return this.adminService.getTransactions({
      merchantId: merchantId ? Number(merchantId) : undefined,
      page: page ? Number(page) : undefined,
      pageSize: pageSize ? Number(pageSize) : undefined,
      status,
      payMethod,
      dateFrom,
      dateTo,
      keyword,
    });
  }

  @Delete('transactions/:orderNo')
  @ApiOperation({ summary: 'Delete a payment order by orderNo' })
  async deleteTransaction(@Param('orderNo') orderNo: string, @Req() req: AuthenticatedRequest) {
    return this.adminService.deleteTransaction(orderNo, this.actorOf(req), clientIp(req));
  }

  @Get('merchant')
  @ApiOperation({ summary: 'Get active merchant credentials' })
  async getMerchant() {
    return this.merchantService.getMerchant();
  }

  @Get('merchants')
  @ApiOperation({ summary: 'List all merchants' })
  async listMerchants() {
    return this.merchantService.listMerchants();
  }

  @Post('merchants')
  @ApiOperation({ summary: 'Create a new merchant' })
  async createMerchant(@Body() body: { name: string }, @Req() req: AuthenticatedRequest) {
    return this.merchantService.createMerchant(body.name, this.actorOf(req), clientIp(req));
  }

  @Patch('merchants/:id')
  @ApiOperation({ summary: 'Update merchant info' })
  async updateMerchant(@Param('id') id: string, @Body() body: { name?: string }, @Req() req: AuthenticatedRequest) {
    return this.merchantService.updateMerchant(Number(id), body, this.actorOf(req), clientIp(req));
  }

  @Post('merchants/:id/toggle')
  @ApiOperation({ summary: 'Toggle merchant active/inactive' })
  async toggleMerchant(@Param('id') id: string, @Req() req: AuthenticatedRequest) {
    return this.merchantService.toggleMerchantActive(Number(id), this.actorOf(req), clientIp(req));
  }

  @Post('merchant/reset-secret')
  @ApiOperation({ summary: 'Reset merchant appSecret' })
  async resetSecret(@Req() req: AuthenticatedRequest) {
    return this.merchantService.resetSecret(this.actorOf(req), clientIp(req));
  }

  @Post('test-pay')
  @ApiOperation({ summary: 'Create a test order for the active merchant and return cashier URL' })
  async testPay(
    @Body() dto: TestPayDto,
    @Req() req: AuthenticatedRequest,
  ) {
    const merchant = await this.merchantService.findActiveMerchant();

    const orderResult = await this.paymentService.createOrder(merchant.id, {
      amount: dto.amount,
      productName: dto.productName?.trim() || 'Sandbox Pay 测试订单',
      payMethod: 'native',
    });

    const baseUrl = `${req.protocol}://${req.get('host')}`;
    return this.nativePayService.createOrder(orderResult.orderNo, baseUrl);
  }

  /**
   * Upload a daily bill CSV from an upstream PSP and reconcile against local
   * orders. The body is the raw CSV string; `provider` selects the parser.
   */
  @Post('reconciliation/upload')
  @ApiOperation({ summary: 'Upload a daily bill CSV and reconcile against local orders' })
  async uploadBill(@Body() dto: UploadBillDto) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(dto.billDate)) {
      throw new BadRequestException('billDate must be YYYY-MM-DD');
    }
    const rows =
      dto.provider === 'alipay'
        ? this.reconciliationService.parseAlipayCsv(dto.csv)
        : this.reconciliationService.parsePayPalCsv(dto.csv);
    return this.reconciliationService.reconcile(dto.provider, dto.billDate, rows);
  }

  /**
   * Admin-initiated refund. JWT-only (no merchant signature) so the dashboard
   * can refund directly. Delegates the full flow to {@link RefundService}
   * without a `merchantId` scope so any merchant's order can be refunded.
   */
  @Post('refund')
  @ApiOperation({ summary: 'Admin: refund a paid order (full or partial)' })
  async adminRefund(@Body() dto: RefundDto, @Req() req: AuthenticatedRequest) {
    return this.refundService.execute(dto, {
      actor: this.actorOf(req),
      ip: clientIp(req),
    });
  }

  @Get('reconciliation')
  @ApiOperation({ summary: 'List reconciliation records' })
  @ApiQuery({ name: 'provider', required: false })
  @ApiQuery({ name: 'billDate', required: false })
  @ApiQuery({ name: 'status', required: false, enum: ReconStatus })
  @ApiQuery({ name: 'page', required: false, type: Number })
  @ApiQuery({ name: 'pageSize', required: false, type: Number })
  async listReconciliation(
    @Query('provider') provider?: string,
    @Query('billDate') billDate?: string,
    @Query('status') status?: ReconStatus,
    @Query('page') page?: string,
    @Query('pageSize') pageSize?: string,
  ) {
    return this.reconciliationService.list({
      provider,
      billDate,
      status,
      page: page ? Number(page) : undefined,
      pageSize: pageSize ? Number(pageSize) : undefined,
    });
  }

  // ── Notifications ────────────────────────────────────────────

  @Get('notifications')
  @ApiOperation({ summary: 'List notification queue with status' })
  @ApiQuery({ name: 'page', required: false, type: Number })
  @ApiQuery({ name: 'pageSize', required: false, type: Number })
  @ApiQuery({ name: 'status', required: false, description: '0=Pending, 1=Success, 2=Failed, 3=Exhausted' })
  async listNotifications(
    @Query('page') page?: string,
    @Query('pageSize') pageSize?: string,
    @Query('status') status?: string,
  ) {
    return this.adminService.getNotificationStatus({
      page: page ? Number(page) : undefined,
      pageSize: pageSize ? Number(pageSize) : undefined,
      status,
    });
  }

  @Post('notifications/:id/replay')
  @ApiOperation({ summary: 'Replay a failed/exhausted notification' })
  async replayNotification(@Param('id') id: string, @Req() req: AuthenticatedRequest) {
    return this.adminService.replayNotification(Number(id), this.actorOf(req), clientIp(req));
  }

  // ── Data Reset (Sandbox) ─────────────────────────────────────

  @Post('reset-data')
  @UseGuards(SandboxGuard)
  @ApiOperation({ summary: '[SANDBOX] Delete all orders, notifications, and audit logs' })
  async resetData(@Req() req: AuthenticatedRequest) {
    return this.adminService.resetData(this.actorOf(req), clientIp(req));
  }

  @Get('audit-logs')
  @ApiOperation({ summary: 'List sensitive-action audit log entries' })
  @ApiQuery({ name: 'action', required: false })
  @ApiQuery({ name: 'actor', required: false })
  @ApiQuery({ name: 'targetId', required: false })
  @ApiQuery({ name: 'page', required: false, type: Number })
  @ApiQuery({ name: 'pageSize', required: false, type: Number })
  async listAuditLogs(
    @Query('action') action?: string,
    @Query('actor') actor?: string,
    @Query('targetId') targetId?: string,
    @Query('page') page?: string,
    @Query('pageSize') pageSize?: string,
  ) {
    return this.auditService.list({
      action,
      actor,
      targetId,
      page: page ? Number(page) : undefined,
      pageSize: pageSize ? Number(pageSize) : undefined,
    });
  }

  // ── Site Settings ──────────────────────────────────────────

  @Get('settings')
  @ApiOperation({ summary: 'Get all site settings' })
  async getSettings() {
    return this.siteSettingsService.getAll();
  }

  @Post('settings')
  @ApiOperation({ summary: 'Update site settings (batch)' })
  async updateSettings(@Body() body: Record<string, string>, @Req() req: AuthenticatedRequest) {
    const allowed = this.siteSettingsService.getAllowedKeys();
    const invalid = Object.keys(body).filter(k => !allowed.has(k));
    if (invalid.length > 0) {
      throw new BadRequestException(`不允许的配置项: ${invalid.join(', ')}`);
    }
    await this.siteSettingsService.setMany(body);
    await this.auditService.log({
      action: 'update_settings',
      actor: this.actorOf(req),
      targetType: 'settings',
      ip: clientIp(req),
      detail: { keys: Object.keys(body) },
    });
    return this.siteSettingsService.getAll();
  }
}
