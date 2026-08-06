import { useState } from 'react'
import { Button, TextField, InputGroup, Card } from '@heroui/react'
import { Zap, HandCoins, ArrowRight, Loader2 } from 'lucide-react'
import { api } from '../../utils/api'
import { showToast as toast } from '../../utils/toast'

/**
 * Authenticated test-pay panel for the admin dashboard. Creates a real order
 * against the logged-in admin's active merchant and redirects to the native
 * cashier. Backed by `POST /api/admin/test-pay`.
 */
export default function TestPayPanel() {
  const [amount, setAmount] = useState('88.88')
  const [productName, setProductName] = useState('Sandbox Pay 测试订单')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const numAmount = parseFloat(amount)
    if (!Number.isFinite(numAmount) || numAmount <= 0) {
      toast.error('请输入有效的测试金额')
      return
    }
    if (!productName.trim()) {
      toast.error('请输入商品名称')
      return
    }

    setSubmitting(true)
    try {
      // Server returns the same envelope native-pay createOrder yields:
      // `{ code, data: cashierUrl }`.
      const payload = await api.post<{ code: number; data: string }>(
        '/admin/test-pay',
        { amount: numAmount, productName },
      )
      const target = payload?.data
      if (!target) {
        toast.error('测试下单失败')
        return
      }
      toast.success('测试下单成功，正在跳转收银台...')
      window.location.href = target
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '网关状态异常'
      toast.error('测试下单失败: ' + message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card id="test-pay-panel" className="p-6 scroll-mt-24">
      <Card.Content className="p-0">
        <div className="flex items-center gap-3 mb-5">
          <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-500 flex items-center justify-center">
            <Zap className="w-5 h-5 fill-current" />
          </div>
          <div>
            <h3 className="font-semibold text-base text-foreground tracking-tight flex items-center gap-2">
              发起测试订单
              <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-500 font-mono font-semibold uppercase">
                Live Demo
              </span>
            </h3>
            <p className="text-xs text-muted mt-0.5">使用当前商户凭据生成一笔真实订单</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3 font-sans">
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-muted tracking-wide block">商品名称</label>
            <TextField aria-label="商品名称" fullWidth isRequired isDisabled={submitting}>
              <InputGroup className="w-full rounded-xl bg-surface border border-border focus-within:border-emerald-500 transition-colors">
                <InputGroup.Input
                  type="text"
                  value={productName}
                  onChange={e => setProductName(e.target.value)}
                  placeholder="例如：高级会员季卡"
                  className="w-full px-3 py-2 bg-transparent text-sm text-foreground focus:outline-none"
                />
              </InputGroup>
            </TextField>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-muted tracking-wide block">支付金额 (元)</label>
            <TextField aria-label="支付金额" fullWidth isRequired isDisabled={submitting}>
              <InputGroup className="w-full rounded-xl bg-surface border border-border focus-within:border-emerald-500 transition-colors">
                <InputGroup.Prefix>
                  <span className="pl-3 text-muted text-sm font-medium font-mono">¥</span>
                </InputGroup.Prefix>
                <InputGroup.Input
                  type="number"
                  step="0.01"
                  min="0.01"
                  value={amount}
                  onChange={e => setAmount(e.target.value)}
                  placeholder="88.88"
                  className="w-full pl-1 pr-3 py-2 bg-transparent text-sm font-mono font-semibold text-foreground focus:outline-none"
                />
              </InputGroup>
            </TextField>
          </div>

          <Button
            type="submit"
            isDisabled={submitting}
            className="w-full py-3 px-4 rounded-xl bg-emerald-500 hover:bg-emerald-600 disabled:opacity-50 text-white font-semibold text-xs transition-all shadow-sm flex items-center justify-center gap-2 mt-2 cursor-pointer h-11"
          >
            {submitting ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                正在创建收银台会话...
              </>
            ) : (
              <>
                <HandCoins className="w-4 h-4" />
                一键发起测试付款
                <ArrowRight className="w-4 h-4" />
              </>
            )}
          </Button>
        </form>
      </Card.Content>
    </Card>
  )
}

