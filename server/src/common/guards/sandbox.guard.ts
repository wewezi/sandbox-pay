import { CanActivate, ExecutionContext, ForbiddenException, Injectable } from '@nestjs/common';
import { SiteSettingsService } from '../services/site-settings.service';

/**
 * Blocks access to sandbox / demo endpoints unless ENABLE_SANDBOX=true in site_settings.
 */
@Injectable()
export class SandboxGuard implements CanActivate {
  constructor(private readonly settings: SiteSettingsService) {}

  canActivate(_context: ExecutionContext): boolean {
    const enabled = this.settings.getBoolean('ENABLE_SANDBOX') ?? false;
    if (!enabled) {
      throw new ForbiddenException('沙箱接口已关闭');
    }
    return true;
  }
}
