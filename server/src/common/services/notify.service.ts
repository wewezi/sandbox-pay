import { Injectable, Logger } from '@nestjs/common';
import { Cron, CronExpression } from '@nestjs/schedule';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { HttpService } from '@nestjs/axios';
import { NotifyQueue, NotifyStatus } from '../../entities/notify-queue.entity';
import { SignatureService } from './signature.service';
import { firstValueFrom } from 'rxjs';
import { errMessage } from '../util/error';

export interface PaymentNotification {
  orderNo: string;
  externalOrderNo?: string;
  status: string;
  amount: number;
  payMethod: string;
  payAt: string;
  thirdPartyTradeNo?: string;
}

@Injectable()
export class NotifyService {
  private readonly logger = new Logger(NotifyService.name);
  private readonly delays = [5, 15, 60, 300, 900];

  constructor(
    @InjectRepository(NotifyQueue)
    private notifyRepository: Repository<NotifyQueue>,
    private httpService: HttpService,
    private signatureService: SignatureService,
  ) {}

  async enqueueNotification(
    notifyUrl: string,
    notification: PaymentNotification,
    appSecret: string,
  ) {
    const body = JSON.stringify(notification);
    const timestamp = Date.now().toString();
    const nonce = Math.random().toString(36).slice(2, 10);
    const payload = this.signatureService.buildPayload(body, timestamp, nonce);
    const signature = this.signatureService.sign(payload, appSecret);

    const item = this.notifyRepository.create({
      orderNo: notification.orderNo,
      url: notifyUrl,
      body,
      signature: JSON.stringify({ timestamp, nonce, sign: signature }),
      status: NotifyStatus.Pending,
    });
    await this.notifyRepository.save(item);
    this.logger.log(`Notification enqueued for order ${notification.orderNo} -> ${notifyUrl}`);
  }

  @Cron(CronExpression.EVERY_MINUTE)
  async handleNotifications() {
    const items = await this.notifyRepository.find({
      where: [
        { status: NotifyStatus.Pending },
        { status: NotifyStatus.Failed },
      ],
      order: { createdAt: 'ASC' },
      take: 50,
    });

    for (const item of items) {
      if (item.status === NotifyStatus.Exhausted) continue;

      const now = new Date();
      if (item.lastAttemptAt) {
        const delay = this.delays[Math.min(item.retryCount, this.delays.length - 1)];
        const nextAttempt = new Date(item.lastAttemptAt.getTime() + delay * 1000);
        if (nextAttempt > now) continue;
      }

      await this.processNotify(item);
    }
  }

  private async processNotify(item: NotifyQueue) {
    item.lastAttemptAt = new Date();
    try {
      const sigInfo = JSON.parse(item.signature);
      const response = await firstValueFrom(
        this.httpService.post(item.url, JSON.parse(item.body), {
          headers: {
            'X-Sandbox-Pay-Timestamp': sigInfo.timestamp,
            'X-Sandbox-Pay-Nonce': sigInfo.nonce,
            'X-Sandbox-Pay-Signature': sigInfo.sign,
            'Content-Type': 'application/json',
          },
          timeout: 5000,
        }),
      );

      if (response.status >= 200 && response.status < 300) {
        item.status = NotifyStatus.Success;
        this.logger.log(`Notification success: ${item.url} (order: ${item.orderNo})`);
      } else {
        this.handleFailure(item, `HTTP ${response.status}`);
      }
    } catch (err: unknown) {
      this.handleFailure(item, errMessage(err));
    }
    await this.notifyRepository.save(item);
  }

  private handleFailure(item: NotifyQueue, error: string) {
    item.retryCount++;
    item.lastError = error;
    if (item.retryCount >= this.delays.length) {
      item.status = NotifyStatus.Exhausted;
      this.logger.error(`Notification exhausted: ${item.url} (order: ${item.orderNo}), Error: ${error}`);
    } else {
      item.status = NotifyStatus.Failed;
      this.logger.warn(`Notification failed: ${item.url} (order: ${item.orderNo}), Retry: ${item.retryCount}, Error: ${error}`);
    }
  }
}
