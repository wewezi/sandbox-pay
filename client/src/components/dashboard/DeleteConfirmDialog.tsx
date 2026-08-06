import { Button, Modal } from '@heroui/react'
import { Trash2, AlertTriangle, RefreshCw } from 'lucide-react'
import type { Transaction } from './types'

interface Props {
  target: Transaction | null
  submitting: boolean
  onClose: () => void
  onSubmit: () => void
}

export default function DeleteConfirmDialog({
  target,
  submitting,
  onClose,
  onSubmit,
}: Props) {
  return (
    <Modal>
      <Modal.Backdrop isOpen={!!target} onOpenChange={(open) => !open && onClose()} variant="blur">
        <Modal.Container size="md">
          <Modal.Dialog className="overflow-hidden rounded-xl border border-rose-500/20 bg-surface p-6 shadow-[0_18px_50px_rgba(0,0,0,0.12)]">
            <Modal.CloseTrigger />
            <Modal.Header className="flex items-center gap-3 text-left pb-4">
              <div className="w-10 h-10 rounded-xl bg-rose-500/10 text-rose-500 flex items-center justify-center shrink-0 border border-rose-500/20">
                <AlertTriangle className="w-5 h-5 fill-rose-500/20" />
              </div>
              <div>
                <Modal.Heading className="text-base font-bold text-foreground tracking-tight">
                  永久删除订单
                </Modal.Heading>
                <p className="text-xs text-muted mt-0.5">此操作无法恢复，将从数据库彻底抹去</p>
              </div>
            </Modal.Header>

            <Modal.Body className="py-2 px-0">
              {target && (
                <div className="p-4 rounded-xl bg-surface/80 border border-border space-y-2 font-mono text-xs">
                  <div className="flex justify-between text-muted">
                    <span>交易单号：</span>
                    <span className="text-foreground font-semibold">{target.orderNo}</span>
                  </div>
                  <div className="flex justify-between text-muted">
                    <span>支付渠道：</span>
                    <span className="text-foreground">{target.channel === 'NATIVE' ? '官方存管' : target.channel}</span>
                  </div>
                  <div className="flex justify-between text-muted pt-1 border-t border-border/60">
                    <span>实付金额：</span>
                    <span className="text-rose-500 font-bold">￥{Number(target.amount || 0).toFixed(2)}</span>
                  </div>
                </div>
              )}
            </Modal.Body>

            <Modal.Footer className="flex items-center justify-end gap-3 pt-6 px-0 pb-0">
              <Button
                variant="ghost"
                onPress={onClose}
                isDisabled={submitting}
                className="px-4 py-2.5 text-xs text-muted hover:text-foreground hover:bg-surface rounded-xl font-medium cursor-pointer"
              >
                取消
              </Button>
              <Button
                onPress={onSubmit}
                isDisabled={submitting}
                className="px-5 py-2.5 text-xs bg-rose-500 hover:bg-rose-600 text-white rounded-xl font-semibold shadow-lg shadow-rose-500/20 disabled:opacity-50 flex items-center gap-1.5 cursor-pointer transition-all active:scale-98"
              >
                {submitting ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Trash2 className="w-3.5 h-3.5" />}
                {submitting ? '正在抹除数据...' : '确认永久删除'}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

