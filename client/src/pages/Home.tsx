import { Link } from 'react-router-dom'
import {
  ArrowRight,
  BellRing,
  CreditCard,
  FileCheck2,
  LayoutDashboard,
  QrCode,
  ShieldCheck,
  Store,
  Wrench,
} from 'lucide-react'
import BrandMark from '../components/BrandMark'

const capabilities = [
  {
    icon: QrCode,
    title: '支付产品',
    description: '聚合多种沙箱支付方式，覆盖完整收银台流程。',
  },
  {
    icon: Wrench,
    title: '运营工具',
    description: '订单、通知和商户能力统一管理，状态清晰可查。',
  },
  {
    icon: CreditCard,
    title: '资金与对账',
    description: '支持退款、渠道账单匹配与交易差异核对。',
  },
  {
    icon: ShieldCheck,
    title: '安全与审计',
    description: '签名鉴权、重放防护和敏感操作审计完整覆盖。',
  },
]

export default function Home() {
  return (
    <div className="min-h-screen bg-white text-[#2b2b2b]">
      <header className="border-b border-[#e8e8e8] bg-white">
        <div className="mx-auto flex h-[86px] max-w-6xl items-center justify-between px-5 sm:px-8">
          <Link to="/" className="flex items-center gap-3">
            <BrandMark size="md" />
            <span className="text-[22px] font-normal tracking-tight text-[#4a4a4a]">WePay</span>
          </Link>

          <Link
            to="/admin"
            className="flex h-10 items-center gap-2 rounded-[3px] bg-[#20b956] px-5 text-[13px] font-medium text-white hover:bg-[#18a94e]"
          >
            <LayoutDashboard className="h-4 w-4" />
            <span className="hidden sm:inline">进入管理后台</span>
            <span className="sm:hidden">管理后台</span>
          </Link>
        </div>
      </header>

      <main>
        <section id="home" className="relative h-[520px] overflow-hidden sm:h-[540px]">
          <img
            src="/images/payment-counter-hero.jpg"
            alt="顾客在商户柜台使用手机完成移动支付"
            className="absolute inset-0 h-full w-full object-cover object-center lg:object-[center_88%]"
          />
          <div className="absolute inset-0 bg-gradient-to-r from-white/75 via-white/30 to-white/5 sm:from-white/80 sm:via-white/25 sm:to-transparent" />

          <div className="relative mx-auto flex h-full max-w-6xl items-start px-5 pt-20 sm:px-8 sm:pt-24">
            <div className="max-w-[600px] text-[#292929] [text-shadow:0_1px_12px_rgba(255,255,255,0.95)]">
              <div className="mb-5 flex items-center gap-2 text-[13px] font-medium text-[#168f48]">
                <Store className="h-4 w-4" /> 支付联调与测试平台
              </div>
              <h1 className="text-[38px] font-semibold leading-[1.2] tracking-tight sm:text-[48px]">WePay 商户助手</h1>
              <p className="mt-5 text-[18px] leading-8 text-[#3d3d3d] sm:text-[22px]">统一沙箱支付流程，让联调更简单、更可靠</p>
              <Link
                to="/admin"
                className="mt-8 inline-flex h-11 items-center gap-2 rounded-[3px] bg-[#20b956] px-6 text-[14px] font-medium text-white [text-shadow:none] hover:bg-[#18a94e]"
              >
                进入商户平台 <ArrowRight className="h-4 w-4" />
              </Link>
            </div>
          </div>

          <div className="absolute inset-x-0 bottom-0 bg-black/55 text-white">
            <div className="mx-auto flex h-[52px] max-w-6xl items-center justify-between gap-4 px-5 text-[12px] sm:px-8">
              <div className="flex min-w-0 items-center gap-3">
                <BellRing className="h-4 w-4 shrink-0" />
                <span className="truncate">WePay 仅用于开发联调，所有交易均为模拟数据</span>
              </div>
              <a href="#capabilities" className="flex shrink-0 items-center gap-1 text-white/90 hover:text-white">查看能力 <ArrowRight className="h-3.5 w-3.5" /></a>
            </div>
          </div>
        </section>

        <section id="capabilities" className="bg-white px-5 py-16 sm:px-8 sm:py-20">
          <div className="mx-auto max-w-6xl">
            <div className="text-center">
              <h2 className="text-[28px] font-semibold tracking-tight">平台开放能力</h2>
              <div className="mx-auto mt-4 h-[3px] w-12 bg-[#20b956]" />
              <p className="mt-4 text-[13px] text-[#888]">覆盖从支付受理到后台运营的完整沙箱流程</p>
            </div>

            <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              {capabilities.map(item => (
                <article key={item.title} className="min-h-[230px] border border-[#eeeeee] bg-white px-6 py-8 text-center transition-shadow hover:shadow-[0_6px_18px_rgba(0,0,0,0.08)]">
                  <div className="mx-auto flex h-24 w-24 items-center justify-center rounded-full bg-[#fafafa] text-[#6f6f6f]">
                    <item.icon className="h-10 w-10" strokeWidth={1.5} />
                  </div>
                  <h3 className="mt-6 text-[17px] font-semibold">{item.title}</h3>
                  <p className="mt-3 text-[12px] leading-5 text-[#888]">{item.description}</p>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section id="security" className="border-t border-[#eeeeee] bg-[#f7f8f9] px-5 py-14 sm:px-8">
          <div className="mx-auto grid max-w-6xl gap-8 sm:grid-cols-3">
            {[
              { icon: ShieldCheck, title: '安全鉴权', text: '请求签名与防重放校验' },
              { icon: FileCheck2, title: '交易可追溯', text: '完整状态与操作审计记录' },
              { icon: CreditCard, title: '沙箱隔离', text: '不产生任何真实资金扣款' },
            ].map(item => (
              <div key={item.title} className="flex items-center gap-4">
                <item.icon className="h-7 w-7 shrink-0 text-[#20b956]" strokeWidth={1.6} />
                <div><h3 className="text-sm font-semibold">{item.title}</h3><p className="mt-1 text-[12px] text-[#888]">{item.text}</p></div>
              </div>
            ))}
          </div>
        </section>
      </main>

      <footer className="border-t border-[#e8e8e8] bg-white px-5 py-7 text-center text-[11px] text-[#999]">
        WePay © {new Date().getFullYear()} · 支付接入沙箱
      </footer>
    </div>
  )
}
