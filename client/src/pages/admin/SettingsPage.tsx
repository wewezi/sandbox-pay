import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card } from '@heroui/react'
import {
  Save, Loader2, DollarSign, TrendingUp,
  Mail, Smartphone, CreditCard, Shield,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'

interface SettingField {
  key: string
  label: string
  type: 'text' | 'number' | 'toggle'
  placeholder?: string
  desc?: string
}

interface SettingGroup {
  title: string
  icon: typeof DollarSign
  color: string
  fields: SettingField[]
}

const SETTING_GROUPS: SettingGroup[] = [
  {
    title: '沙箱与安全',
    icon: Shield,
    color: 'text-emerald-500',
    fields: [
      { key: 'ENABLE_SANDBOX', label: '启用沙箱', type: 'toggle', desc: '开启后可使用测试下单、沙箱确认等接口' },
      { key: 'THROTTLE_TTL_MS', label: '限流窗口 (ms)', type: 'number', placeholder: '60000', desc: '每个 IP 在此时间窗口内的最大请求数' },
      { key: 'THROTTLE_LIMIT', label: '限流次数', type: 'number', placeholder: '60', desc: '每个窗口内允许的最大请求数' },
    ],
  },
  {
    title: '手续费',
    icon: DollarSign,
    color: 'text-blue-500',
    fields: [
      { key: 'FEE_RATE_ALIPAY', label: '支付宝费率', type: 'text', placeholder: '0.006', desc: '小数表示，0.006 = 0.6%' },
      { key: 'FEE_RATE_PAYPAL', label: 'PayPal 费率', type: 'text', placeholder: '0.044' },
      { key: 'FEE_RATE_NATIVE', label: '官方存管费率', type: 'text', placeholder: '0' },
      { key: 'CHANNEL_COST_ALIPAY', label: '支付宝渠道成本', type: 'text', placeholder: '0.006' },
      { key: 'CHANNEL_COST_PAYPAL', label: 'PayPal 渠道成本', type: 'text', placeholder: '0.044' },
      { key: 'CHANNEL_COST_NATIVE', label: '官方存管渠道成本', type: 'text', placeholder: '0' },
      { key: 'FEE_MIN_CENTS', label: '手续费下限 (分)', type: 'number', placeholder: '0' },
    ],
  },
  {
    title: '汇率',
    icon: TrendingUp,
    color: 'text-violet-500',
    fields: [
      { key: 'EXCHANGE_RATE_PROVIDER', label: '汇率来源', type: 'text', placeholder: 'open-er-api', desc: 'open-er-api 或 static' },
      { key: 'EXCHANGE_RATE_CACHE_TTL_MS', label: '缓存时间 (ms)', type: 'number', placeholder: '3600000' },
      { key: 'FALLBACK_CNY_TO_USD', label: '兜底汇率 CNY→USD', type: 'text', placeholder: '0.14' },
      { key: 'EXCHANGE_RATE_MARGIN', label: '汇率加点', type: 'text', placeholder: '1', desc: '1 = 不加点，0.98 = 缩水 2%' },
    ],
  },
  {
    title: '支付宝',
    icon: CreditCard,
    color: 'text-blue-600',
    fields: [
      { key: 'ALIPAY_APP_ID', label: 'App ID', type: 'text', placeholder: '' },
      { key: 'ALIPAY_SERVER_URL', label: '网关地址', type: 'text', placeholder: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do' },
    ],
  },
  {
    title: 'PayPal',
    icon: CreditCard,
    color: 'text-indigo-500',
    fields: [
      { key: 'PAYPAL_CLIENT_ID', label: 'Client ID', type: 'text', placeholder: '' },
      { key: 'PAYPAL_ENVIRONMENT', label: '环境', type: 'text', placeholder: 'sandbox', desc: 'sandbox 或 live' },
    ],
  },
  {
    title: '官方存管',
    icon: Smartphone,
    color: 'text-amber-500',
    fields: [
      { key: 'NATIVE_PAY_QR_URL', label: '收款二维码 URL', type: 'text', placeholder: '' },
      { key: 'NATIVE_PAY_ACCOUNT_NAME', label: '收款账户名', type: 'text', placeholder: '' },
      { key: 'NATIVE_PAY_ACCOUNT_NO', label: '收款账号', type: 'text', placeholder: '' },
      { key: 'NATIVE_PAY_BANK_NAME', label: '开户行', type: 'text', placeholder: '' },
    ],
  },
  {
    title: '邮件通知',
    icon: Mail,
    color: 'text-rose-500',
    fields: [
      { key: 'SMTP_HOST', label: 'SMTP 服务器', type: 'text', placeholder: 'smtp.qq.com' },
      { key: 'SMTP_PORT', label: '端口', type: 'number', placeholder: '587' },
      { key: 'SMTP_USER', label: '用户名', type: 'text', placeholder: '' },
      { key: 'SMTP_FROM', label: '发件人', type: 'text', placeholder: '' },
    ],
  },
]

export default function SettingsPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [settings, setSettings] = useState<Record<string, string>>({})
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [refreshing, setRefreshing] = useState(false)
  const [savingGroup, setSavingGroup] = useState<string | null>(null)

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const data = await api.get<Record<string, string>>('/admin/settings')
      if (data) {
        setSettings(data)
        setDraft(data)
      }
      if (isRefresh) toast.success('设置已刷新')
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

  const updateDraft = (key: string, value: string) => {
    setDraft(prev => ({ ...prev, [key]: value }))
  }

  const handleSaveGroup = async (group: SettingGroup) => {
    setSavingGroup(group.title)
    try {
      const changes: Record<string, string> = {}
      for (const field of group.fields) {
        if (draft[field.key] !== settings[field.key]) {
          changes[field.key] = draft[field.key] ?? ''
        }
      }
      if (Object.keys(changes).length === 0) {
        toast.info('没有需要保存的更改')
        return
      }
      const updated = await api.post<Record<string, string>>('/admin/settings', changes)
      if (updated) {
        setSettings(updated)
        setDraft(updated)
        toast.success(`「${group.title}」设置已保存`)
      }
    } catch (err: unknown) {
      toast.error('保存失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setSavingGroup(null)
    }
  }

  const hasGroupChanges = (group: SettingGroup) => {
    return group.fields.some(f => draft[f.key] !== settings[f.key])
  }

  return (
    <AdminLayout
      title="站点设置"
      subtitle="业务运行参数，修改后即时生效，无需重启服务"
      refreshing={refreshing}
      onRefresh={handleRefresh}
    >
      <div className="space-y-6">
        {SETTING_GROUPS.map(group => {
          const Icon = group.icon
          const hasChanges = hasGroupChanges(group)
          const isSaving = savingGroup === group.title

          return (
            <Card key={group.title} className="p-6">
              <Card.Content className="p-0">
                <div className="flex items-center justify-between mb-5">
                  <div className="flex items-center gap-3">
                    <div className={`w-9 h-9 rounded-lg bg-surface ${group.color} flex items-center justify-center`}>
                      <Icon className="w-4.5 h-4.5" />
                    </div>
                    <div>
                      <h3 className="text-sm font-semibold text-foreground">{group.title}</h3>
                    </div>
                  </div>
                  <Button
                    size="sm"
                    onPress={() => handleSaveGroup(group)}
                    isDisabled={!hasChanges || isSaving}
                    className={`px-4 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 cursor-pointer h-auto min-h-0 ${
                      hasChanges
                        ? 'bg-emerald-500 hover:bg-emerald-600 text-white'
                        : 'bg-surface text-muted cursor-default'
                    }`}
                  >
                    {isSaving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
                    {isSaving ? '保存中...' : '保存'}
                  </Button>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                  {group.fields.map(field => (
                    <div key={field.key} className="space-y-1.5">
                      <label className="text-xs font-medium text-muted flex items-center gap-1.5">
                        {field.label}
                        {draft[field.key] !== settings[field.key] && (
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
                        )}
                      </label>
                      {field.type === 'toggle' ? (
                        <button
                          type="button"
                          onClick={() => updateDraft(field.key, draft[field.key] === 'true' ? 'false' : 'true')}
                          className={`
                            relative inline-flex h-8 w-14 items-center rounded-full transition-colors cursor-pointer
                            ${draft[field.key] === 'true' ? 'bg-emerald-500' : 'bg-surface-secondary'}
                          `}
                        >
                          <span
                            className={`
                              inline-block h-6 w-6 rounded-full bg-white shadow-sm transition-transform
                              ${draft[field.key] === 'true' ? 'translate-x-7' : 'translate-x-1'}
                            `}
                          />
                        </button>
                      ) : (
                        <input
                          type={field.type === 'number' ? 'number' : 'text'}
                          value={draft[field.key] ?? ''}
                          onChange={e => updateDraft(field.key, e.target.value)}
                          placeholder={field.placeholder}
                          className="w-full h-9 rounded-lg border border-border bg-surface text-xs font-mono text-foreground px-3 focus:outline-none focus:border-emerald-500 transition-colors"
                        />
                      )}
                      {field.desc && (
                        <p className="text-[10px] text-muted">{field.desc}</p>
                      )}
                    </div>
                  ))}
                </div>
              </Card.Content>
            </Card>
          )
        })}
      </div>
    </AdminLayout>
  )
}
