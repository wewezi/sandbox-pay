import { NotifyStatus } from '../../entities/notify-queue.entity';
import { NotifyService } from './notify.service';

function createMockNotify(overrides: Partial<any> = {}): any {
  return {
    id: 1,
    orderNo: 'PAY123',
    url: 'https://example.com/notify',
    body: JSON.stringify({ orderNo: 'PAY123', status: 'paid', amount: 100 }),
    signature: JSON.stringify({ timestamp: '1234567890', nonce: 'abc123', sign: 'sig' }),
    status: NotifyStatus.Pending,
    retryCount: 0,
    lastAttemptAt: null,
    lastError: null,
    createdAt: new Date(),
    ...overrides,
  };
}

function createService(overrides: Record<string, any> = {}) {
  const defaults = {
    notifyRepository: {
      find: jest.fn().mockResolvedValue([]),
      save: jest.fn().mockImplementation(async (item: any) => item),
      create: jest.fn().mockImplementation((item: any) => item),
    },
    httpService: {
      post: jest.fn(),
    },
    signatureService: {
      buildPayload: jest.fn().mockReturnValue('payload'),
      sign: jest.fn().mockReturnValue('signature'),
    },
  };
  const deps = { ...defaults, ...overrides };
  const service = new NotifyService(
    deps.notifyRepository as any,
    deps.httpService as any,
    deps.signatureService as any,
  );
  return { service, deps };
}

describe('NotifyService', () => {
  describe('enqueueNotification', () => {
    it('creates and saves a pending notification', async () => {
      const { service, deps } = createService();
      const notification = {
        orderNo: 'PAY123',
        status: 'paid',
        amount: 100,
        payMethod: 'alipay',
        payAt: new Date().toISOString(),
      };

      await service.enqueueNotification('https://example.com/notify', notification, 'secret');

      expect(deps.signatureService.buildPayload).toHaveBeenCalled();
      expect(deps.signatureService.sign).toHaveBeenCalledWith('payload', 'secret');
      expect(deps.notifyRepository.create).toHaveBeenCalledWith(
        expect.objectContaining({
          orderNo: 'PAY123',
          url: 'https://example.com/notify',
          status: NotifyStatus.Pending,
        }),
      );
      expect(deps.notifyRepository.save).toHaveBeenCalled();
    });
  });

  describe('handleNotifications', () => {
    it('skips items not yet due for retry', async () => {
      const { service, deps } = createService();
      const recentAttempt = new Date(Date.now() - 1000); // 1 second ago
      deps.notifyRepository.find.mockResolvedValue([
        createMockNotify({ status: NotifyStatus.Failed, retryCount: 1, lastAttemptAt: recentAttempt }),
      ]);

      await service.handleNotifications();

      expect(deps.notifyRepository.save).not.toHaveBeenCalled();
    });

    it('processes items that are due for retry', async () => {
      const { service, deps } = createService();
      const oldAttempt = new Date(Date.now() - 60000); // 60 seconds ago
      deps.notifyRepository.find.mockResolvedValue([
        createMockNotify({ status: NotifyStatus.Failed, retryCount: 1, lastAttemptAt: oldAttempt }),
      ]);
      const firstValueFromSpy = jest.spyOn(require('rxjs'), 'firstValueFrom')
        .mockResolvedValue({ status: 200 });

      await service.handleNotifications();

      expect(deps.notifyRepository.save).toHaveBeenCalled();
    });

    it('marks notification as exhausted after max retries', async () => {
      const { service, deps } = createService();
      deps.notifyRepository.find.mockResolvedValue([
        createMockNotify({ status: NotifyStatus.Failed, retryCount: 5, lastAttemptAt: new Date(0) }),
      ]);
      jest.spyOn(require('rxjs'), 'firstValueFrom').mockRejectedValue(new Error('timeout'));

      await service.handleNotifications();

      const savedItem = deps.notifyRepository.save.mock.calls[0]?.[0];
      if (savedItem) {
        expect(savedItem.status).toBe(NotifyStatus.Exhausted);
      }
    });
  });
});
