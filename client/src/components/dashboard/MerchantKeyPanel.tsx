import { Button, Card } from '@heroui/react'
import { ShieldCheck, Key, Copy, Check, RefreshCw } from 'lucide-react'
import type { MerchantInfo } from './types'

interface Props {
  merchant: MerchantInfo
  copiedKey: boolean
  onCopy: (text: string, type: string) => void
  onResetSecret: () => void
}

export default function MerchantKeyPanel({ merchant, copiedKey, onCopy, onResetSecret }: Props) {
  return (
    <Card className="p-6">
      <Card.Content className="p-0">
        <div className="flex items-center gap-3 mb-6">
          <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-500 flex items-center justify-center">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <h3 className="font-semibold text-base text-foreground tracking-tight">商户密钥鉴权</h3>
            <p className="text-xs text-muted">调用 Sandbox Pay API 必备鉴权参数</p>
          </div>
        </div>

        <div className="space-y-5 font-mono text-xs">
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-muted flex items-center gap-1.5">
                <Key className="w-3.5 h-3.5 text-muted" />
                AppKey (应用 ID)
              </span>
              <Button
                size="sm"
                variant="ghost"
                onPress={() => onCopy(merchant.appKey, 'AppKey')}
                className="text-xs text-emerald-500 hover:text-emerald-600 flex items-center gap-1 px-2 py-1 h-auto min-w-0 min-h-0 cursor-pointer"
              >
                {copiedKey ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                复制
              </Button>
            </div>
            <div className="p-3.5 rounded-xl bg-surface/80 text-xs select-all text-foreground font-medium tracking-wider overflow-x-auto">
              {merchant.appKey}
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-muted flex items-center gap-1.5">
                <Key className="w-3.5 h-3.5 text-muted" />
                AppSecret (签名私钥)
              </span>
              <Button
                size="sm"
                variant="ghost"
                onPress={() => onCopy(merchant.appSecret, 'AppSecret')}
                className="text-xs text-muted hover:text-foreground flex items-center gap-1 px-2 py-1 h-auto min-w-0 min-h-0 cursor-pointer"
              >
                <Copy className="w-3.5 h-3.5" />
                复制
              </Button>
            </div>
            <div className="p-3.5 rounded-xl bg-surface/80 text-muted flex items-center justify-between">
              <span className="truncate max-w-[180px]">
                {merchant.appSecret ? merchant.appSecret.substring(0, 16) + '...' : '••••••••••••••••'}
              </span>
              <span className="text-[10px] bg-background text-muted px-2 py-0.5 rounded font-mono shadow-xs">
                HMAC-256
              </span>
            </div>
          </div>
        </div>

        <div className="mt-6 pt-6 border-t border-border">
          <Button
            onPress={onResetSecret}
            variant="secondary"
            className="w-full py-3 bg-surface hover:bg-surface-secondary text-foreground font-medium rounded-xl text-xs flex items-center justify-center gap-2 cursor-pointer"
          >
            <RefreshCw className="w-3.5 h-3.5 text-muted" />
            重新生成私钥
          </Button>
        </div>
      </Card.Content>
    </Card>
  )
}

