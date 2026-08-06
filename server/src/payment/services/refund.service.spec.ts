import { BadRequestException, NotFoundException, ConflictException } from '@nestjs/common';
import { RefundService } from './refund.service';
import { OrderStatus } from '../../entities/payment-order.entity';

function createMockOrder(overrides: Partial<any> = {}): any {
  return {
    id: 'test-id',
    orderNo: 'PAY20250101001',
    amount: 10000,
    refundedAmount: 0,
    status: OrderStatus.Paid,
    payMethod: 'alipay',
    merchantId: 1,
    thirdPartyTradeNo: 'ALI_TRADE_123',
    foreignAmount: null,
    foreignCurrency: null,
    notifyUrl: 'https://example.com/notify',
    merchant: { id: 1, appSecret: 'encrypted' },
    ...overrides,
  };
}

function createService(overrides: Record<string, any> = {}) {
  const defaults = {
    orderRepository: { findOne: jest.fn(), save: jest.fn() },
    paymentService: {
      getOrderForRefund: jest.fn(),
      markRefunding: jest.fn().mockResolvedValue(undefined),
      markRefunded: jest.fn().mockResolvedValue(undefined),
      markRefundFailed: jest.fn().mockResolvedValue(undefined),
    },
    alipayService: { refund: jest.fn().mockResolvedValue('ALI_REFUND_123') },
    paypalService: { refund: jest.fn().mockResolvedValue('PAYPAL_REFUND_123') },
    auditService: { log: jest.fn().mockResolvedValue(undefined) },
  };
  const deps = { ...defaults, ...overrides };
  const service = new RefundService(
    deps.orderRepository as any,
    deps.paymentService as any,
    deps.alipayService as any,
    deps.paypalService as any,
    deps.auditService as any,
  );
  return { service, deps };
}

describe('RefundService', () => {
  describe('execute', () => {
    it('throws when orderNo is missing', async () => {
      const { service } = createService();
      await expect(service.execute({ orderNo: '', amount: 10 }, { actor: 'admin' }))
        .rejects.toThrow(BadRequestException);
    });

    it('throws when amount is missing', async () => {
      const { service } = createService();
      await expect(service.execute({ orderNo: 'PAY123', amount: 0 }, { actor: 'admin' }))
        .rejects.toThrow(BadRequestException);
    });

    it('throws when order not found (admin path)', async () => {
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(null);
      await expect(service.execute({ orderNo: 'PAY123', amount: 10 }, { actor: 'admin' }))
        .rejects.toThrow(NotFoundException);
    });

    it('throws when order status is not Paid (admin path)', async () => {
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(
        createMockOrder({ status: OrderStatus.Pending }),
      );
      await expect(service.execute({ orderNo: 'PAY123', amount: 10 }, { actor: 'admin' }))
        .rejects.toThrow(ConflictException);
    });

    it('throws when refund amount exceeds refundable balance', async () => {
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(
        createMockOrder({ amount: 1000, refundedAmount: 800 }),
      );
      await expect(service.execute({ orderNo: 'PAY123', amount: 5 }, { actor: 'admin' }))
        .rejects.toThrow(BadRequestException);
    });

    it('successfully refunds via alipay', async () => {
      const order = createMockOrder();
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(order);

      const result = await service.execute({ orderNo: 'PAY123', amount: 50 }, { actor: 'admin' });

      expect(result.orderNo).toBe('PAY123');
      expect(result.refundTradeNo).toBe('ALI_REFUND_123');
      expect(result.currency).toBe('CNY');
      expect(deps.paymentService.markRefunding).toHaveBeenCalledWith('PAY123');
      expect(deps.alipayService.refund).toHaveBeenCalledWith(
        'PAY20250101001',
        5000,
        undefined,
      );
      expect(deps.paymentService.markRefunded).toHaveBeenCalled();
      expect(deps.auditService.log).toHaveBeenCalled();
    });

    it('successfully refunds via native', async () => {
      const order = createMockOrder({ payMethod: 'native', thirdPartyTradeNo: null });
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(order);

      const result = await service.execute({ orderNo: 'PAY123', amount: 50 }, { actor: 'admin' });

      expect(result.currency).toBe('CNY');
      expect(result.refundTradeNo).toContain('SP_REFUND_');
    });

    it('rolls back to Paid when upstream refund fails', async () => {
      const order = createMockOrder();
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(order);
      deps.alipayService.refund.mockRejectedValue(new Error('Alipay API timeout'));

      await expect(service.execute({ orderNo: 'PAY123', amount: 50 }, { actor: 'admin' }))
        .rejects.toThrow(BadRequestException);

      expect(deps.paymentService.markRefundFailed).toHaveBeenCalledWith('PAY123', 'Alipay API timeout');
    });

    it('rolls back when upstream returns null tradeNo', async () => {
      const order = createMockOrder();
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(order);
      deps.alipayService.refund.mockResolvedValue(null);

      await expect(service.execute({ orderNo: 'PAY123', amount: 50 }, { actor: 'admin' }))
        .rejects.toThrow(BadRequestException);

      expect(deps.paymentService.markRefundFailed).toHaveBeenCalled();
    });

    it('throws for unsupported payMethod', async () => {
      const order = createMockOrder({ payMethod: 'bitcoin' });
      const { service, deps } = createService();
      deps.orderRepository.findOne.mockResolvedValue(order);

      await expect(service.execute({ orderNo: 'PAY123', amount: 50 }, { actor: 'admin' }))
        .rejects.toThrow(BadRequestException);
    });

    it('uses paymentService.getOrderForRefund when merchantId is provided', async () => {
      const order = createMockOrder();
      const { service, deps } = createService();
      deps.paymentService.getOrderForRefund.mockResolvedValue(order);

      await service.execute({ orderNo: 'PAY123', amount: 50 }, { actor: 'merchant:1', merchantId: 1 });

      expect(deps.paymentService.getOrderForRefund).toHaveBeenCalledWith('PAY123', 1);
      expect(deps.orderRepository.findOne).not.toHaveBeenCalled();
    });
  });
});
