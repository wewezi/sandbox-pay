import {
  WebSocketGateway,
  WebSocketServer,
  SubscribeMessage,
  OnGatewayConnection,
  OnGatewayDisconnect,
  WsException,
} from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';
import { Logger } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { PaymentOrder } from '../entities/payment-order.entity';
import { isCorsAllowed } from '../common/util/cors-origin';

@WebSocketGateway({
  cors: {
    origin: (origin: string | undefined, callback: (err: Error | null, allow?: boolean) => void) => {
      if (isCorsAllowed(origin)) {
        callback(null, true);
      } else {
        callback(new Error(`WebSocket CORS blocked: ${origin}`));
      }
    },
    credentials: true,
  },
  namespace: 'payment',
})
export class PaymentGateway implements OnGatewayConnection, OnGatewayDisconnect {
  @WebSocketServer()
  server: Server;

  private readonly logger = new Logger(PaymentGateway.name);

  constructor(
    @InjectRepository(PaymentOrder)
    private readonly orderRepository: Repository<PaymentOrder>,
  ) {}

  handleConnection(client: Socket) {
    this.logger.log(`Client connected: ${client.id}`);
  }

  handleDisconnect(client: Socket) {
    this.logger.log(`Client disconnected: ${client.id}`);
  }

  @SubscribeMessage('subscribeOrder')
  async handleSubscribeOrder(client: Socket, orderNo: string) {
    // 校验订单存在且处于可订阅状态（pending/refunding 才需要实时推送）
    const order = await this.orderRepository.findOne({
      where: { orderNo },
      select: ['id', 'orderNo', 'status'],
    });

    if (!order) {
      throw new WsException('订单不存在');
    }

    client.join(`order:${orderNo}`);
    this.logger.log(`Client ${client.id} subscribed to order: ${orderNo}`);
    return { status: 'subscribed', orderNo };
  }

  notifyPaymentStatus(orderNo: string, status: string) {
    this.server.to(`order:${orderNo}`).emit('paymentStatus', { orderNo, status });
  }
}
