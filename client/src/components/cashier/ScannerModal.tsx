import { Button, Modal } from '@heroui/react'
import { QrCode, ShieldCheck, Zap, RefreshCw, Check } from 'lucide-react'
import { api } from '../../utils/api'
import { showToast as toast } from '../../utils/toast'
import { useEffect, useState } from 'react'

interface Props {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  orderNo: string
  amount: number
  onPaid: () => void
}

export default function ScannerModal({ isOpen, onOpenChange, orderNo, amount, onPaid }: Props) {
  const [step, setStep] = useState<'scanning' | 'confirm' | 'success'>('scanning')
  const [paying, setPaying] = useState(false)

  useEffect(() => {
    if (!isOpen) return
    const timer = setTimeout(() => setStep('confirm'), 1200)
    return () => clearTimeout(timer)
  }, [isOpen])

  const handleOpen = (open: boolean) => {
    onOpenChange(open)
  }

  const handleConfirm = async () => {
    setPaying(true)
    try {
      await api.post('/native-pay/sandbox-confirm', {
        orderNo,
        walletUser: 'app_scan@sandbox-pay.local',
        walletPass: '123456',
      })
      setStep('success')
      setTimeout(() => {
        onOpenChange(false)
        onPaid()
      }, 1200)
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '该单据不可通过存管网关核销'
      toast.error('模拟支付失败：' + message)
    } finally {
      setPaying(false)
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={isOpen} onOpenChange={handleOpen} variant="blur">
        <Modal.Container size="xs">
          <Modal.Dialog className="flex flex-col items-center overflow-hidden rounded-[36px] border-[6px] border-[#202020] bg-[#f5f5f5] p-5 text-[#191919] shadow-2xl dark:bg-[#181c19] dark:text-white">
            <div className="absolute left-1/2 top-0 flex h-5 w-24 -translate-x-1/2 items-center justify-center rounded-b-xl bg-[#202020]">
              <div className="h-1 w-10 rounded-full bg-[#555]" />
            </div>

            <Modal.Header className="flex w-full items-center justify-between border-b border-black/[0.06] px-0 pb-3 pt-4 text-xs dark:border-white/10">
              <Modal.Heading className="font-semibold">Sandbox Pay 钱包 · 扫一扫</Modal.Heading>
              <Modal.CloseTrigger className="h-auto min-h-0 min-w-0 cursor-pointer bg-transparent p-1 text-sm font-bold text-[#888] hover:text-[#191919] dark:hover:text-white" />
            </Modal.Header>

            <Modal.Body className="flex min-h-[280px] w-full flex-col items-center justify-center space-y-4 px-0 py-8 text-center">
              {step === 'scanning' && (
                <div className="space-y-6 flex flex-col items-center">
                  <div className="relative flex h-40 w-40 items-center justify-center rounded-2xl border-2 border-dashed border-emerald-500 bg-white p-2 dark:bg-black/20">
                    <div className="absolute inset-x-2 top-1/2 h-0.5 bg-emerald-500 shadow-[0_0_10px_rgba(7,193,96,0.45)]" />
                    <QrCode className="h-20 w-20 text-emerald-500/40" />
                  </div>
                  <div className="text-xs text-[#888]">
                    正在识别付款二维码…
                  </div>
                </div>
              )}

              {step === 'confirm' && (
                <div className="space-y-5 w-full px-2 animate-scale-up">
                  <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400">
                    <ShieldCheck className="w-8 h-8" />
                  </div>
                  <div className="space-y-1">
                    <div className="text-xs text-[#888]">支付给</div>
                    <div className="text-base font-bold tracking-tight">Sandbox Pay 测试商户</div>
                    <div className="pt-1 text-2xl font-bold tabular-nums">¥ {amount.toFixed(2)}</div>
                  </div>
                  <div className="space-y-2 rounded-xl bg-white p-3.5 text-left font-mono text-[11px] dark:bg-black/20">
                    <div className="flex justify-between border-b border-black/[0.06] pb-1.5 text-[#888] dark:border-white/10">
                      <span>付款单号：</span>
                      <span className="max-w-[140px] truncate text-foreground">{orderNo}</span>
                    </div>
                    <div className="flex justify-between text-[#888]">
                      <span>扣款账户：</span>
                      <span className="font-bold text-emerald-600 dark:text-emerald-400">沙箱钱包 (可用 ¥88,888.00)</span>
                    </div>
                  </div>
                  <Button
                    isDisabled={paying}
                    onPress={handleConfirm}
                    className="flex w-full cursor-pointer items-center justify-center gap-2 rounded-lg bg-emerald-500 py-3.5 text-xs font-bold text-white shadow-[0_8px_20px_rgba(7,193,96,0.18)] hover:bg-emerald-600 disabled:opacity-50"
                  >
                    {paying ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Zap className="w-4 h-4 fill-current" />}
                    <span>确认支付 ¥{amount.toFixed(2)}</span>
                  </Button>
                </div>
              )}

              {step === 'success' && (
                <div className="space-y-4 flex flex-col items-center animate-fade-in">
                  <div className="flex h-16 w-16 items-center justify-center rounded-full bg-emerald-500 text-white">
                    <Check className="w-8 h-8 stroke-[3]" />
                  </div>
                  <div className="text-base font-bold tracking-wide">支付成功</div>
                  <div className="font-mono text-xs text-emerald-600 dark:text-emerald-400">正在同步订单状态…</div>
                </div>
              )}
            </Modal.Body>

            <Modal.Footer className="flex justify-center pb-0 pt-2 px-0">
              <div className="h-1 w-28 rounded-full bg-[#c8c8c8] dark:bg-[#555]" />
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
