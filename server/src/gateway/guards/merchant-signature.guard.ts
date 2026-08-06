import {
  Injectable,
  CanActivate,
  ExecutionContext,
  UnauthorizedException,
} from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { Merchant } from '../../entities/merchant.entity';
import { SignatureService } from '../../common/services/signature.service';
import { EncryptionService } from '../../common/services/encryption.service';
import { NonceStore } from '../../common/services/nonce-store.service';

const TIMESTAMP_WINDOW_MS = 5 * 60 * 1000; // 5 minutes
const NONCE_TTL_MS = TIMESTAMP_WINDOW_MS + 60 * 1000; // window + small slack

@Injectable()
export class MerchantSignatureGuard implements CanActivate {
  constructor(
    @InjectRepository(Merchant)
    private merchantRepository: Repository<Merchant>,
    private signatureService: SignatureService,
    private encryptionService: EncryptionService,
    private nonceStore: NonceStore,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();
    const appKey = request.headers['x-sandbox-pay-appkey'];
    const timestamp = request.headers['x-sandbox-pay-timestamp'];
    const nonce = request.headers['x-sandbox-pay-nonce'];
    const signature = request.headers['x-sandbox-pay-signature'];

    if (!appKey || !timestamp || !nonce || !signature) {
      throw new UnauthorizedException('缺少安全请求头');
    }

    const merchant = await this.merchantRepository.findOne({
      where: { appKey, isActive: true },
    });
    if (!merchant) {
      throw new UnauthorizedException('无效的 AppKey 或商户已停用');
    }

    // Time window check (5 minutes)
    const now = Date.now();
    const requestTime = parseInt(timestamp, 10);
    if (isNaN(requestTime) || Math.abs(now - requestTime) > TIMESTAMP_WINDOW_MS) {
      throw new UnauthorizedException('时间戳已过期或无效');
    }

    // Replay protection: nonce must be unique within the time window, scoped per merchant
    const nonceKey = `${appKey}:${nonce}`;
    if (!(await this.nonceStore.tryConsume(nonceKey, NONCE_TTL_MS))) {
      throw new UnauthorizedException('Nonce 重复，疑似重放攻击');
    }

    // Canonical payload: GET signs the sorted query string (excluding signature headers),
    // POST/PUT/DELETE sign the raw JSON body.
    const bodyStr =
      request.method === 'GET'
        ? this.signatureService.canonicalizeQuery(request.query)
        : JSON.stringify(request.body || {});

    const payload = this.signatureService.buildPayload(bodyStr, timestamp, nonce);
    const secret = this.encryptionService.decrypt(merchant.appSecret);
    const isValid = this.signatureService.verify(payload, secret, signature);

    if (!isValid) {
      throw new UnauthorizedException('签名验证失败');
    }

    request['merchant'] = merchant;
    return true;
  }
}
