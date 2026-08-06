import { useState } from 'react'
import {
  AlertTriangle,
  Check,
  Copy,
  KeyRound,
  LockKeyhole,
  Search,
  Send,
  Undo2,
} from 'lucide-react'
import AdminLayout from '../../components/admin/AdminLayout'
import { showToast as toast } from '../../utils/toast'

const endpoints = [
  { method: 'POST', path: '/gateway/pay', description: '统一创建支付订单', icon: Send },
  { method: 'POST', path: '/gateway/alipay/pay', description: '创建支付宝订单', icon: Send },
  { method: 'POST', path: '/gateway/paypal/pay', description: '创建 PayPal 订单', icon: Send },
  { method: 'POST', path: '/gateway/native/pay', description: '创建 Sandbox 钱包订单', icon: Send },
  { method: 'GET', path: '/gateway/query?orderNo={orderNo}', description: '查询订单状态', icon: Search },
  { method: 'POST', path: '/gateway/refund', description: '发起全额或部分退款', icon: Undo2 },
]

const requestExample = `{
  "amount": 1299.00,
  "productName": "专业版年度订阅",
  "payMethod": "native",
  "externalOrderNo": "MERCHANT-20260715-001",
  "returnUrl": "https://merchant.example.com/payment/return",
  "notifyUrl": "https://merchant.example.com/payment/notify"
}`

const signingExample = `import crypto from 'node:crypto'

const body = JSON.stringify(requestBody)
const timestamp = Date.now().toString()
const nonce = crypto.randomUUID()
const payload = \`${'${body}'}&timestamp=${'${timestamp}'}&nonce=${'${nonce}'}\`
const signature = crypto
  .createHmac('sha256', appSecret)
  .update(payload)
  .digest('hex')

const response = await fetch(baseUrl + '/gateway/pay', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Sandbox-Pay-AppKey': appKey,
    'X-Sandbox-Pay-Timestamp': timestamp,
    'X-Sandbox-Pay-Nonce': nonce,
    'X-Sandbox-Pay-Signature': signature,
  },
  body,
})`

const responseExample = `{
  "code": 200,
  "msg": "success",
  "data": {
    "type": "url",
    "data": "https://pay.example.com/cashier?orderNo=..."
  }
}`

function CodeBlock({ code, label }: { code: string; label: string }) {
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      toast.success(`${label}已复制`)
      setTimeout(() => setCopied(false), 1600)
    } catch {
      toast.error('复制失败，请手动复制')
    }
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border bg-[#18181b] text-[#e4e4e7]">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-2.5 text-[10px] text-[#a1a1aa]">
        <span>{label}</span>
        <button type="button" onClick={copy} className="flex items-center gap-1.5 hover:text-white">
          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <pre className="overflow-x-auto p-4 font-mono text-[11px] leading-5"><code>{code}</code></pre>
    </div>
  )
}

export default function IntegrationDocsPage() {
  const baseUrl = `${window.location.origin}/v1/api`

  return (
    <AdminLayout title="接入文档" subtitle="商户网关接入说明，仅限控制台内部使用">
      <div className="mx-auto max-w-5xl space-y-6">
        <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-xs leading-5 text-amber-900 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-100">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <div><span className="font-semibold">内部资料。</span> 文档包含接口路径和签名规则，请勿复制到公开页面或对外分发。</div>
        </div>

        <section className="rounded-xl border border-border bg-surface p-5 sm:p-6">
          <div className="flex items-center gap-3">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-surface-secondary text-muted"><LockKeyhole className="h-4 w-4" /></span>
            <div>
              <h2 className="text-sm font-semibold">开始接入</h2>
              <p className="mt-0.5 text-[11px] text-muted">所有金额单位为元，响应统一使用 JSON 包装结构</p>
            </div>
          </div>
          <div className="mt-5 grid gap-3 sm:grid-cols-2">
            <div className="rounded-lg bg-surface-secondary p-4">
              <div className="text-[10px] text-muted">Base URL</div>
              <code className="mt-2 block break-all text-xs font-medium">{baseUrl}</code>
            </div>
            <div className="rounded-lg bg-surface-secondary p-4">
              <div className="text-[10px] text-muted">凭证位置</div>
              <div className="mt-2 text-xs font-medium">开发沙箱 → 商户密钥鉴权</div>
            </div>
          </div>
        </section>

        <section className="rounded-xl border border-border bg-surface p-5 sm:p-6">
          <div className="flex items-center gap-3">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-surface-secondary text-muted"><KeyRound className="h-4 w-4" /></span>
            <div>
              <h2 className="text-sm font-semibold">请求签名</h2>
              <p className="mt-0.5 text-[11px] text-muted">每次请求必须使用独立 Nonce，时间戳有效窗口为 5 分钟</p>
            </div>
          </div>

          <div className="mt-5 overflow-hidden rounded-lg border border-border text-xs">
            {[
              ['X-Sandbox-Pay-AppKey', '商户 AppKey'],
              ['X-Sandbox-Pay-Timestamp', '当前毫秒时间戳'],
              ['X-Sandbox-Pay-Nonce', '当前请求的唯一随机串'],
              ['X-Sandbox-Pay-Signature', 'HMAC-SHA256 签名十六进制小写结果'],
            ].map(([name, description], index, items) => (
              <div key={name} className={`grid gap-1 px-4 py-3 sm:grid-cols-[240px_1fr] ${index !== items.length - 1 ? 'border-b border-border' : ''}`}>
                <code className="text-[11px] font-medium">{name}</code>
                <span className="text-muted">{description}</span>
              </div>
            ))}
          </div>

          <div className="mt-4 rounded-lg bg-surface-secondary p-4 text-[11px] leading-5">
            <div className="font-semibold">待签名字符串</div>
            <code className="mt-1 block break-all text-muted">POST：JSON.stringify(body)&amp;timestamp={'{timestamp}'}&amp;nonce={'{nonce}'}</code>
            <code className="mt-1 block break-all text-muted">GET：按键名排序的查询串&amp;timestamp={'{timestamp}'}&amp;nonce={'{nonce}'}</code>
          </div>
        </section>

        <section className="rounded-xl border border-border bg-surface p-5 sm:p-6">
          <h2 className="text-sm font-semibold">网关端点</h2>
          <div className="mt-4 overflow-hidden rounded-lg border border-border">
            {endpoints.map((endpoint, index) => {
              const Icon = endpoint.icon
              return (
                <div key={`${endpoint.method}-${endpoint.path}`} className={`flex items-center gap-3 px-4 py-3 text-xs ${index !== endpoints.length - 1 ? 'border-b border-border' : ''}`}>
                  <Icon className="h-3.5 w-3.5 shrink-0 text-muted" />
                  <span className={`w-10 shrink-0 font-mono text-[10px] font-semibold ${endpoint.method === 'GET' ? 'text-emerald-700 dark:text-emerald-400' : 'text-blue-700 dark:text-blue-400'}`}>{endpoint.method}</span>
                  <code className="min-w-0 flex-1 truncate text-[11px]">{endpoint.path}</code>
                  <span className="hidden text-muted sm:block">{endpoint.description}</span>
                </div>
              )
            })}
          </div>
        </section>

        <section className="grid gap-5 lg:grid-cols-2">
          <div>
            <h2 className="mb-3 text-sm font-semibold">下单请求</h2>
            <CodeBlock label="request.json" code={requestExample} />
          </div>
          <div>
            <h2 className="mb-3 text-sm font-semibold">统一响应</h2>
            <CodeBlock label="response.json" code={responseExample} />
          </div>
        </section>

        <section>
          <h2 className="mb-3 text-sm font-semibold">Node.js 签名示例</h2>
          <CodeBlock label="create-payment.mjs" code={signingExample} />
        </section>

        <section className="rounded-xl border border-border bg-surface p-5 sm:p-6">
          <h2 className="text-sm font-semibold">常见错误码</h2>
          <div className="mt-4 grid gap-3 text-xs sm:grid-cols-2 lg:grid-cols-3">
            {[
              ['400', '请求字段校验失败'],
              ['401', '签名、AppKey 或时间戳无效'],
              ['403', '当前环境或权限不允许操作'],
              ['404', '订单或资源不存在'],
              ['409', '重复订单或状态冲突'],
              ['429', '请求频率超过限制'],
            ].map(([code, description]) => (
              <div key={code} className="flex gap-3 rounded-lg bg-surface-secondary p-3">
                <code className="font-semibold">{code}</code>
                <span className="text-muted">{description}</span>
              </div>
            ))}
          </div>
        </section>
      </div>
    </AdminLayout>
  )
}
