import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Button } from '@heroui/react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import MerchantKeyPanel from '../../components/dashboard/MerchantKeyPanel'
import TestPayPanel from '../../components/dashboard/TestPayPanel'
import type { MerchantInfo } from '../../components/dashboard/types'

export default function SandboxPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [merchant, setMerchant] = useState<MerchantInfo>({
    name: 'WePay Merchant',
    appKey: 'sp_sandbox_...',
    appSecret: '••••••••••••••••••••••••••••',
  })
  const [copiedKey, setCopiedKey] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const data = await api.get<MerchantInfo>('/admin/merchant')
      if (data) setMerchant(data)
      if (isRefresh) toast.success('数据已刷新')
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        logout()
        navigate('/login')
      } else {
        toast.error('加载失败: ' + (err instanceof Error ? err.message : '网络异常'))
      }
    } finally {
      if (isRefresh) setRefreshing(false)
    }
  }, [logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const handleResetSecret = async () => {
    try {
      const newMerchant = await api.post<MerchantInfo>('/admin/merchant/reset-secret')
      if (newMerchant) {
        setMerchant(newMerchant)
        toast.success('已生成全新的商户签名私钥 (AppSecret)')
      }
    } catch (err: unknown) {
      toast.error('重置密钥失败: ' + (err instanceof Error ? err.message : '网络异常'))
    }
  }

  const handleCopy = (text: string, type: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(true)
    toast.success(`${type} 已复制到剪贴板`)
    setTimeout(() => setCopiedKey(false), 2000)
  }

  const handleResetData = async () => {
    if (!confirm('确定要清除所有沙箱数据吗？此操作不可恢复。')) return
    try {
      const result = await api.post<{ deleted: { orders: number; notifications: number; auditLogs: number } }>('/admin/reset-data')
      if (result) {
        toast.success(`已清除: ${result.deleted.orders} 笔订单, ${result.deleted.notifications} 条通知, ${result.deleted.auditLogs} 条日志`)
      }
    } catch (err: unknown) {
      toast.error('清除失败: ' + (err instanceof Error ? err.message : '网络异常'))
    }
  }

  return (
    <AdminLayout
      title="开发沙箱"
      subtitle="密钥管理、测试下单与数据重置"
      refreshing={refreshing}
      onRefresh={handleRefresh}
    >
      {/* Warning banner */}
      <Card className="p-4 mb-6 border-amber-500/20 bg-amber-500/5">
        <Card.Content className="p-0">
          <div className="flex items-start gap-3">
            <AlertTriangle className="w-4 h-4 text-amber-500 mt-0.5 shrink-0" />
            <div className="text-xs text-foreground">
              <span className="font-semibold">沙箱环境</span> — 以下功能仅用于开发联调测试。
              所有操作均会产生真实数据库记录，请谨慎使用「清除数据」功能。
            </div>
          </div>
        </Card.Content>
      </Card>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
        <MerchantKeyPanel
          merchant={merchant}
          copiedKey={copiedKey}
          onCopy={handleCopy}
          onResetSecret={handleResetSecret}
        />
        <TestPayPanel />
      </div>

      {/* Data reset section */}
      <Card className="p-6 mt-6">
        <Card.Content className="p-0">
          <div className="flex items-center gap-3 mb-4">
            <div className="w-10 h-10 rounded-xl bg-rose-500/10 text-rose-500 flex items-center justify-center">
              <AlertTriangle className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-semibold text-base text-foreground tracking-tight">数据重置</h3>
              <p className="text-xs text-muted mt-0.5">清除所有订单、通知队列和审计日志</p>
            </div>
          </div>
          <p className="text-xs text-muted mb-4">
            此操作将永久删除数据库中所有支付订单、通知队列记录和审计日志。商户数据不会被清除。
            请仅在开发/测试环境中使用。
          </p>
          <Button
            onPress={handleResetData}
            className="px-4 py-2.5 rounded-xl bg-rose-500 hover:bg-rose-600 text-white text-xs font-semibold flex items-center gap-2 cursor-pointer h-auto"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            清除所有沙箱数据
          </Button>
        </Card.Content>
      </Card>
    </AdminLayout>
  )
}
