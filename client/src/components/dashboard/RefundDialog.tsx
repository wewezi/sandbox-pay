import { Button, TextField, InputGroup, Modal } from '@heroui/react'
import { Undo2, RefreshCw } from 'lucide-react'
import type { Transaction } from './types'

interface Props {
  target: Transaction | null
  amount: string
  setAmount: (v: string) => void
  reason: string
  setReason: (v: string) => void
  submitting: boolean
  onClose: () => void
  onSubmit: () => void
}

export default function RefundDialog({
  target,
  amount,
  setAmount,
  reason,
  setReason,
  submitting,
  onClose,
  onSubmit,
}: Props) {
  return (
    <Modal>
      <Modal.Backdrop isOpen={!!target} onOpenChange={(open) => !open && onClose()} variant="blur">
        <Modal.Container size="md">
          <Modal.Dialog className="rounded-xl border border-border bg-surface p-6 shadow-[0_18px_50px_rgba(0,0,0,0.12)]">
            <Modal.CloseTrigger />
            <Modal.Header className="flex flex-col items-start gap-1 text-left pb-4">
              <Modal.Heading className="text-base font-semibold text-foreground tracking-tight flex items-center gap-2">
                <Undo2 className="w-4 h-4 text-rose-500" />
                发起退款
              </Modal.Heading>
              {target && <p className="text-xs text-muted font-mono">{target.orderNo}</p>}
            </Modal.Header>

            <Modal.Body className="py-2 px-0">
              {target && (
                <div className="space-y-4">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted">订单金额</span>
                    <span className="font-mono font-semibold text-foreground">￥{Number(target.amount || 0).toFixed(2)}</span>
                  </div>
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted">已退金额</span>
                    <span className="font-mono text-foreground">￥{Number(target.refundedAmount || 0).toFixed(2)}</span>
                  </div>
                  <div className="flex items-center justify-between text-xs border-b border-border pb-3">
                    <span className="text-muted">可退余额</span>
                    <span className="font-mono font-semibold text-emerald-500">
                      ￥{(Number(target.amount || 0) - Number(target.refundedAmount || 0)).toFixed(2)}
                    </span>
                  </div>

                  <div className="block">
                    <span className="text-xs font-medium text-muted mb-1.5 block">退款金额 (CNY)</span>
                    <TextField aria-label="退款金额" fullWidth isDisabled={submitting}>
                      <InputGroup className="w-full bg-surface border border-border rounded-lg focus-within:ring-2 focus-within:ring-rose-500/30 focus-within:border-rose-500 transition-all">
                        <InputGroup.Input
                          type="number"
                          step="0.01"
                          min="0.01"
                          value={amount}
                          onChange={e => setAmount(e.target.value)}
                          className="w-full px-3 py-2 text-sm font-mono bg-transparent focus:outline-none disabled:opacity-50"
                          placeholder="0.00"
                        />
                      </InputGroup>
                    </TextField>
                  </div>

                  <div className="block">
                    <span className="text-xs font-medium text-muted mb-1.5 block">退款原因（可选）</span>
                    <TextField aria-label="退款原因" fullWidth isDisabled={submitting}>
                      <InputGroup className="w-full bg-surface border border-border rounded-lg focus-within:ring-2 focus-within:ring-rose-500/30 focus-within:border-rose-500 transition-all">
                        <InputGroup.Input
                          type="text"
                          maxLength={255}
                          value={reason}
                          onChange={e => setReason(e.target.value)}
                          className="w-full px-3 py-2 text-sm bg-transparent focus:outline-none disabled:opacity-50"
                          placeholder="用户取消订单"
                        />
                      </InputGroup>
                    </TextField>
                  </div>
                </div>
              )}
            </Modal.Body>

            <Modal.Footer className="flex items-center justify-end gap-2 pt-6 px-0 pb-0">
              <Button
                variant="ghost"
                onPress={onClose}
                isDisabled={submitting}
                className="px-4 py-2 text-xs text-muted hover:bg-surface rounded-lg cursor-pointer"
              >
                取消
              </Button>
              <Button
                onPress={onSubmit}
                isDisabled={submitting}
                className="px-4 py-2 text-xs bg-rose-500 hover:bg-rose-600 text-white rounded-lg font-medium disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
              >
                {submitting ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Undo2 className="w-3.5 h-3.5" />}
                {submitting ? '退款中...' : '确认退款'}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

