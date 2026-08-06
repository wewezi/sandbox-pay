import { Controller, Post, Req, Res } from '@nestjs/common';
import type { Request, Response } from 'express';
import { AlipayService } from '../gateways/alipay.service';
import { ApiTags, ApiOperation, ApiExcludeEndpoint } from '@nestjs/swagger';

@ApiTags('Alipay')
@Controller('api/alipay')
export class AlipayController {
  constructor(private alipayService: AlipayService) {}

  /**
   * Alipay async payment notification.
   *
   * Alipay POSTs application/x-www-form-urlencoded and expects a plain-text
   * "success" or "failure" body (NOT JSON). We bypass the global transform
   * interceptor by using `res.send()` directly.
   */
  @Post('notify')
  @ApiOperation({ summary: 'Alipay async payment notification (form-urlencoded)' })
  @ApiExcludeEndpoint()
  async handleNotify(@Req() req: Request, @Res() res: Response) {
    const params: Record<string, string> = { ...(req.body || {}) };
    const result = await this.alipayService.handleNotify(params);
    res
      .status(200)
      .type('text/plain')
      .send(result === 'success' ? 'success' : 'failure');
  }
}
