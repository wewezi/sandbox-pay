import { LitElement, html, svg, nothing, type TemplateResult } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { api, yuan, percent, count, qs, isAbort, requireAuth, dateTime, RequestController, STATUS_LABEL } from './app';
import type { AdminDashboard, DashboardChannel, DashboardSummary, DashboardTrend } from './api-types';
import { sharedStyles, pageStyles } from './styles/shared';
import { dashboardStyles } from './styles/dashboard';

const CHANNEL_LABEL: Readonly<Record<string, string>> = { native: '自有通道', alipay: '支付宝', paypal: 'PayPal' };
type ChartMetric = 'amount' | 'totalCount';
type Comparison = { label: string; kind: string };
const shortDate = (date: string) => date.slice(5).replace('-', '/');

class SpDashboardPage extends LitElement {
  static override properties = {
    authorized: { state: true }, days: { state: true }, dashboard: { state: true },
    loading: { state: true }, errorMessage: { state: true }, metric: { state: true }, selectedDate: { state: true },
  };
  static override styles = [sharedStyles, pageStyles, dashboardStyles];
  declare private authorized: boolean;
  declare private days: 7 | 30;
  declare private dashboard: AdminDashboard | null;
  declare private loading: boolean;
  declare private errorMessage: string;
  declare private metric: ChartMetric;
  declare private selectedDate: string;
  private activeRequest: AbortSignal | null = null;
  private readonly requests = new RequestController(this, () => void this.load());

  constructor() {
    super();
    this.authorized = false;
    this.days = 7;
    this.dashboard = null;
    this.loading = false;
    this.errorMessage = '';
    this.metric = 'amount';
    this.selectedDate = '';
  }

  override connectedCallback() {
    super.connectedCallback();
    this.authorized = requireAuth();
    if (this.authorized) void this.load();
  }

  override disconnectedCallback() {
    this.activeRequest = null;
    this.loading = false;
    super.disconnectedCallback();
  }

  private async load() {
    if (!this.isConnected || !this.authorized) return;
    const request = this.requests.next('dashboard');
    this.activeRequest = request;
    this.loading = true;
    this.errorMessage = '';
    try {
      const result = await api.get<AdminDashboard>('/admin/dashboard' + qs({
        days: this.days, utcOffset: -new Date().getTimezoneOffset(),
      }), { signal: request });
      if (request.aborted || this.activeRequest !== request || !this.isConnected) return;
      this.dashboard = result;
      if (!result.trend.some((day) => day.date === this.selectedDate)) this.selectedDate = result.dateTo;
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.activeRequest === request && this.isConnected) {
        this.errorMessage = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (this.activeRequest === request) {
        this.activeRequest = null;
        this.loading = false;
      }
      this.requests.release('dashboard', request);
    }
  }

  private changePeriod(days: 7 | 30) {
    if (days === this.days) return;
    this.days = days;
    this.dashboard = null;
    this.selectedDate = '';
    void this.load();
  }

  private comparison(current: number, previous: number, favorable = true): Comparison {
    const prefix = `较前 ${this.dashboard?.days ?? this.days} 天`;
    if (current === previous) return { label: `${prefix}持平`, kind: '' };
    if (previous === 0) return { label: `前 ${this.dashboard?.days ?? this.days} 天为 0`, kind: '' };
    const increase = current > previous;
    return {
      label: `${prefix} ${increase ? '↑' : '↓'} ${percent(Math.abs(current - previous) / previous)}`,
      kind: favorable ? increase ? 'positive' : 'negative' : '',
    };
  }

  private rateComparison(current: number | null, previous: number | null): Comparison {
    if (current === null || previous === null) return { label: '暂无可比数据', kind: '' };
    const delta = (current - previous) * 100;
    if (delta === 0) return { label: `较前 ${this.dashboard?.days ?? this.days} 天持平`, kind: '' };
    return {
      label: `较前 ${this.dashboard?.days ?? this.days} 天 ${delta > 0 ? '+' : '−'}${Math.abs(delta).toFixed(1)} 个百分点`,
      kind: delta > 0 ? 'positive' : 'negative',
    };
  }

  /** 客单价：成交金额 / 成交笔数。没成交时没有意义，返回 null。 */
  private averageOrder(summary: DashboardSummary): number | null {
    return summary.successCount > 0 ? summary.amount / summary.successCount : null;
  }

  private averageComparison(current: number | null, previous: number | null): Comparison {
    if (current === null || previous === null) return { label: '暂无可比数据', kind: '' };
    return this.comparison(current, previous);
  }

  private ordersLink(extra: Record<string, string | number> = {}) {
    const dashboard = this.dashboard;
    return '/orders' + qs({
      ...(dashboard ? { dateFrom: dashboard.dateFrom, dateTo: dashboard.dateTo, utcOffset: dashboard.utcOffset } : {}),
      ...extra,
    });
  }

  private statCard(label: string, value: string, hint: string, comparison: Comparison, icon: TemplateResult) {
    return html`<article class="stat">
      <div class="k"><span>${label}</span><svg class="stat-icon" viewBox="0 0 24 24" aria-hidden="true">${icon}</svg></div>
      <div class="v">${value}</div>
      <div class="hint">${hint}</div>
      <div class=${`comparison ${comparison.kind}`}>${comparison.label}</div>
    </article>`;
  }

  private summaryCards(dashboard: AdminDashboard) {
    const current = dashboard.summary;
    const previous = dashboard.previous;
    const average = this.averageOrder(current);
    return html`<div class="stats dashboard-stats" aria-label="期间统计">
      ${this.statCard('成交金额', yuan(current.amount), `成交 ${count(current.successCount)} 笔`,
        this.comparison(current.amount, previous.amount), svg`<path d="M12 3v18M16 7H9a3 3 0 0 0 0 6h6a3 3 0 0 1 0 6H7"/>`)}
      ${this.statCard('订单数', count(current.totalCount), `已支付 ${count(current.successCount)} 笔`,
        this.comparison(current.totalCount, previous.totalCount), svg`<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 8h6M9 12h6M9 16h4"/>`)}
      ${this.statCard('支付成功率', current.successRate === null ? '—' : percent(current.successRate),
        current.totalCount ? '成功订单 / 全部订单' : '本期暂无订单', this.rateComparison(current.successRate, previous.successRate),
        svg`<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>`)}
      ${this.statCard('平均客单价', average === null ? '—' : yuan(average),
        average === null ? '本期暂无成交' : `成交 ${count(current.successCount)} 笔的均值`,
        this.averageComparison(average, this.averageOrder(previous)),
        svg`<path d="M3 12V5a2 2 0 0 1 2-2h7l9 9-9 9-9-9Z"/><circle cx="7.5" cy="7.5" r="1.5"/>`)}
      ${this.statCard('累计退款', yuan(current.refundedAmount), '本期订单已确认的退款金额',
        this.comparison(current.refundedAmount, previous.refundedAmount, false), svg`<path d="M4 9h10a6 6 0 0 1 0 12M4 9l4-4M4 9l4 4"/>`)}
      ${this.statCard('手续费', yuan(current.fee), '本期订单的手续费合计',
        this.comparison(current.fee, previous.fee), svg`<path d="M5 19 19 5"/><circle cx="7.5" cy="7.5" r="2.5"/><circle cx="16.5" cy="16.5" r="2.5"/>`)}
    </div>`;
  }

  private chart(dashboard: AdminDashboard) {
    const days = dashboard.trend;
    const maxValue = Math.max(1, ...days.map((day) => day[this.metric]));
    const width = 700;
    const bottom = 196;
    const top = 14;
    const x = (index: number) => 10 + index * (width - 20) / Math.max(1, days.length - 1);
    const y = (day: DashboardTrend) => bottom - day[this.metric] / maxValue * (bottom - top);
    const selectedIndex = Math.max(0, days.findIndex((day) => day.date === this.selectedDate));
    const selected = days[selectedIndex];
    const line = days.map((day, index) => `${index === 0 ? 'M' : 'L'}${x(index)},${y(day)}`).join(' ');
    const axisValue = (value: number) => this.metric === 'amount' ? yuan(value) : count(Math.round(value));
    return html`<section class="panel trend-panel" aria-labelledby="trend-title">
      <div class="panel-head trend-head">
        <div class="panel-title"><h2 id="trend-title">交易趋势</h2><p>查看每天的成交和订单变化</p></div>
        <div class="segmented" role="group" aria-label="趋势指标">
          <button aria-pressed=${this.metric === 'amount' ? 'true' : 'false'} @click=${() => { this.metric = 'amount'; }}>成交金额</button>
          <button aria-pressed=${this.metric === 'totalCount' ? 'true' : 'false'} @click=${() => { this.metric = 'totalCount'; }}>订单数</button>
        </div>
      </div>
      <div class="chart-readout" aria-live="polite" aria-atomic="true">
        <span class="chart-date">${selected?.date ?? '—'}</span>
        <strong class="chart-amount">${selected ? this.metric === 'amount' ? yuan(selected.amount) : `${count(selected.totalCount)} 笔` : '—'}</strong>
        <span class="chart-count">${selected ? `订单 ${count(selected.totalCount)} 笔 · 成功 ${count(selected.successCount)} 笔` : ''}</span>
      </div>
      <div class="chart-grid">
        <div class="y-axis" aria-hidden="true"><span>${axisValue(maxValue)}</span><span>${axisValue(maxValue / 2)}</span><span>${axisValue(0)}</span></div>
        <div>
          <svg class="chart" viewBox="0 0 700 210" preserveAspectRatio="none" role="group"
            aria-label=${`近 ${dashboard.days} 天${this.metric === 'amount' ? '成交金额' : '订单数'}趋势，可聚焦每个日期查看数值`}>
            <defs><linearGradient id="dashboard-trend-fill" x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stop-color="var(--brand)" stop-opacity="0.2"/><stop offset="100%" stop-color="var(--brand)" stop-opacity="0"/>
            </linearGradient></defs>
            ${[top, (top + bottom) / 2, bottom].map((height) => svg`<line class="chart-gridline" x1="0" x2="700" y1=${height} y2=${height}/>`)}
            ${days.length ? svg`<path class="chart-area" d=${`${line} L${x(days.length - 1)},${bottom} L${x(0)},${bottom} Z`}/>
              <path class="chart-line" d=${line}/>` : nothing}
            ${selected ? svg`<line class="chart-guide" x1=${x(selectedIndex)} x2=${x(selectedIndex)} y1=${top} y2=${bottom}/>` : nothing}
            ${days.map((day, index) => svg`<rect class="chart-hit" x=${index === 0 ? 0 : (x(index - 1) + x(index)) / 2} y="0"
              width=${index === 0 || index === days.length - 1 ? (width - 20) / Math.max(1, days.length - 1) / 2 + 10 : (width - 20) / Math.max(1, days.length - 1)}
              height="210" tabindex="0" role="button" aria-pressed=${day.date === this.selectedDate ? 'true' : 'false'}
              aria-label=${`${day.date}，成交 ${yuan(day.amount)}，订单 ${count(day.totalCount)} 笔，成功 ${count(day.successCount)} 笔`}
              @pointerenter=${() => { this.selectedDate = day.date; }} @focus=${() => { this.selectedDate = day.date; }}
              @click=${() => { this.selectedDate = day.date; }} @keydown=${(event: KeyboardEvent) => {
                if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); this.selectedDate = day.date; }
              }}><title>${day.date} · 成交 ${yuan(day.amount)} · 订单 ${count(day.totalCount)} 笔</title></rect>`)}
            ${selected ? svg`<ellipse class="chart-point" cx=${x(selectedIndex)} cy=${y(selected)} rx="4" ry="4"/>` : nothing}
          </svg>
          <div class="x-axis" aria-hidden="true"><span>${shortDate(dashboard.dateFrom)}</span><span>${days.length ? shortDate(days[Math.floor((days.length - 1) / 2)].date) : ''}</span><span>${shortDate(dashboard.dateTo)}</span></div>
        </div>
      </div>
      <p class="chart-footnote">${dashboard.summary.totalCount === 0 ? '本期暂无订单，创建测试订单后即可查看趋势。' : '悬停、点击或使用 Tab 键查看每日数据。'}</p>
    </section>`;
  }

  private task(label: string, detail: string, value: number, href: string, kind = 'attention') {
    return html`<a class=${`task ${value > 0 ? kind : ''}`} href=${href}>
      <span class="task-info"><span class="task-label">${label}</span><span class="task-detail">${detail}</span></span>
      <span class="task-value"><strong>${count(value)}</strong><span aria-hidden="true">›</span></span>
    </a>`;
  }

  private tasks(dashboard: AdminDashboard) {
    const tasks = dashboard.tasks;
    const attention = tasks.refundingOrders + tasks.failedNotifications + tasks.exhaustedNotifications;
    return html`<section class="panel task-panel" aria-labelledby="tasks-title">
      <div class="panel-head"><div class="panel-title"><h2 id="tasks-title">待处理事项</h2><p>全部历史订单与通知</p></div>
        <span class=${`badge ${attention ? 'warn' : 'ok'}`}>${attention ? '需要关注' : '暂无异常'}</span></div>
      ${this.task('待支付订单', '查看未完成的支付', tasks.pendingOrders, '/orders?status=0')}
      ${this.task('退款处理中', '继续查询上游退款结果', tasks.refundingOrders, '/orders?status=3')}
      ${this.task('通知等待发送', '查看回调投递', tasks.pendingNotifications, '/notifications?tab=callbacks&status=0', '')}
      ${this.task('通知发送失败', '查看错误与重试记录', tasks.failedNotifications, '/notifications?tab=callbacks&status=2', 'alert')}
      ${this.task('通知重试耗尽', '检查回调地址后重新投递', tasks.exhaustedNotifications, '/notifications?tab=callbacks&status=3', 'alert')}
    </section>`;
  }

  private channelRow(channel: DashboardChannel, dashboard: AdminDashboard) {
    const method = channel.payMethod.toLowerCase();
    const share = dashboard.summary.totalCount ? channel.totalCount / dashboard.summary.totalCount : 0;
    return html`<tr>
      <td><a class="channel-label channel-link" href=${this.ordersLink({ payMethod: channel.payMethod })}>
        <span class=${`channel-dot ${method}`} aria-hidden="true"></span>${CHANNEL_LABEL[method] ?? channel.payMethod}</a></td>
      <td class="num">${count(channel.totalCount)}</td>
      <td class="num">${yuan(channel.amount)}</td>
      <td class="num">${channel.successRate === null ? '—' : percent(channel.successRate)}</td>
      <td class="num">${yuan(channel.refundedAmount)}</td>
      <td class="num channel-share">${percent(share)}<svg class="share-bar" viewBox="0 0 68 5" aria-hidden="true">
        <rect class="share-track" width="68" height="5" rx="2.5"/><rect class="share-value" width=${share * 68} height="5" rx="2.5"/>
      </svg></td>
    </tr>`;
  }

  private channels(dashboard: AdminDashboard) {
    return html`<section class="panel channel-panel" aria-labelledby="channels-title">
      <div class="panel-head"><div class="panel-title"><h2 id="channels-title">渠道表现</h2><p>所选期间各支付渠道的成交情况</p></div>
        <a class="small" href=${this.ordersLink()}>查看订单 →</a></div>
      <div class="table-scroll"><table><thead><tr><th>渠道</th><th class="num">订单数</th><th class="num">成交金额</th>
        <th class="num">成功率</th><th class="num">已退金额</th><th class="num">订单占比</th></tr></thead>
        <tbody>${repeat(dashboard.channels, (channel) => channel.payMethod, (channel) => this.channelRow(channel, dashboard))}</tbody>
      </table></div>
      ${dashboard.channels.length === 0 ? html`<div class="empty-channel">本期暂无渠道交易数据</div>` : nothing}
    </section>`;
  }

  private connections(dashboard: AdminDashboard) {
    return html`<section class="panel connection-panel" aria-labelledby="connections-title">
      <div class="panel-head"><div class="panel-title"><h2 id="connections-title">接入概况</h2><p>当前可用的接入凭证</p></div><a class="small" href="/groups">管理 →</a></div>
      <a class="connection-card" href="/groups"><span>启用分组</span><strong>${count(dashboard.tasks.activeGroups)}</strong></a>
      <a class="connection-card" href="/groups"><span>有效 API Key</span><strong>${count(dashboard.tasks.activeKeys)}</strong></a>
      <p class="connection-note">有效 Key 仅统计启用分组下的启用凭证。<br><a href="/sandbox">前往沙箱创建测试订单 →</a></p>
    </section>`;
  }

  private recentOrders(dashboard: AdminDashboard) {
    const orders = dashboard.recentOrders;
    return html`<section class="panel recent-panel" aria-labelledby="recent-title">
      <div class="panel-head"><div class="panel-title"><h2 id="recent-title">最近订单</h2>
        <p>全部订单里最新的 ${orders.length} 笔，不受统计期间影响</p></div>
        <a class="small" href="/orders">查看全部 →</a></div>
      <div class="table-scroll"><table>
        <thead><tr><th>订单号</th><th>商品</th><th>渠道</th><th class="num">金额</th><th>状态</th><th>创建时间</th></tr></thead>
        <tbody>${repeat(orders, (order) => order.orderNo, (order) => {
          const [label, kind] = STATUS_LABEL[order.status] ?? [order.status, ''];
          const method = order.payMethod.toLowerCase();
          return html`<tr>
            <td><a class="mono recent-no" href=${'/orders' + qs({ keyword: order.orderNo })}>${order.orderNo}</a></td>
            <td><span class="recent-product" title=${order.productName}>${order.productName}</span></td>
            <td class="small"><span class="channel-label"><span class=${`channel-dot ${method}`} aria-hidden="true"></span>${CHANNEL_LABEL[method] ?? order.payMethod}</span></td>
            <td class="num">${yuan(order.amount)}</td>
            <td><span class=${kind ? `badge ${kind}` : 'badge'}>${label}</span></td>
            <td class="small muted"><sp-time .datetime=${order.createdAt}></sp-time></td>
          </tr>`;
        })}</tbody>
      </table></div>
      ${orders.length === 0 ? html`<div class="empty-channel">还没有订单，去 <a href="/sandbox">沙箱</a> 创建一笔测试订单。</div>` : nothing}
    </section>`;
  }

  protected override render() {
    if (!this.authorized) return nothing;
    const dashboard = this.dashboard;
    return html`
      <main class="wrap wide" aria-busy=${this.loading ? 'true' : 'false'}>
        <div class="dashboard-head">
          <div class="page-head"><h1>数据看板</h1><p>${dashboard ? `${dashboard.dateFrom} 至 ${dashboard.dateTo}` : '支付、退款与接入情况一览'}</p></div>
          <div class="head-actions"><div class="segmented" role="group" aria-label="统计期间">
            <button aria-pressed=${this.days === 7 ? 'true' : 'false'} @click=${() => this.changePeriod(7)}>近 7 天</button>
            <button aria-pressed=${this.days === 30 ? 'true' : 'false'} @click=${() => this.changePeriod(30)}>近 30 天</button>
          </div><button ?disabled=${this.loading} @click=${() => void this.load()}>${this.loading ? '刷新中…' : '刷新数据'}</button></div>
        </div>
        ${this.errorMessage ? html`<section class="panel dashboard-status" role="alert">
          <div class="status-mark error" aria-hidden="true">!</div><h2>运营数据加载失败</h2><p class="status-copy">${this.errorMessage}</p>
          <button class="primary" ?disabled=${this.loading} @click=${() => void this.load()}>重新加载</button>
          ${dashboard ? html`<p class="status-copy">下方保留 ${dateTime(dashboard.generatedAt)} 的统计结果。</p>` : nothing}
        </section>` : nothing}
        ${dashboard ? html`${this.summaryCards(dashboard)}
          <div class="primary-grid">${this.chart(dashboard)}${this.tasks(dashboard)}</div>
          <div class="lower-grid">${this.channels(dashboard)}${this.connections(dashboard)}</div>
          ${this.recentOrders(dashboard)}
          <footer class="dashboard-footer"><span>按订单创建日期统计；成交包含已退款订单，退款为累计已退金额。</span>
            <span>更新于 <time datetime=${dashboard.generatedAt}>${dateTime(dashboard.generatedAt)}</time></span></footer>`
          : this.loading ? html`<section class="panel dashboard-status" role="status"><div class="status-mark loading" aria-hidden="true">↻</div>
            <h2>正在加载运营数据</h2><p class="status-copy">汇总交易、渠道与待处理事项…</p></section>` : nothing}
      </main>`;
  }
}

customElements.define('sp-dashboard-page', SpDashboardPage);
