import { LitElement, html, nothing, type PropertyValues, type TemplateResult } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { api, dateTime, isAbort, STATUS_LABEL, yuan } from '../app';
import type { AdminOrderDetail, AdminOrderNotification, AdminRefundOperation } from '../api-types';
import { RequestController } from '../controllers/requests';
import { modalStyles, sharedStyles } from '../styles/shared';
import { orderDetailStyles } from '../styles/order-detail';
import { cleanLocationValue, locationSummary, networkLabel } from '../services/location';

const CHANNEL_LABEL: Readonly<Record<string, string>> = { native: '自有通道', alipay: '支付宝', paypal: 'PayPal' };
const NOTIFY_LABEL: Readonly<Record<number, readonly [string, string]>> = {
  0: ['待发送', 'info'], 1: ['发送成功', 'ok'], 2: ['失败重试中', 'warn'], 3: ['重试耗尽', 'err'],
};
const REFUND_LABEL: Readonly<Record<string, readonly [string, string]>> = {
  processing: ['处理中', 'info'], pending: ['等待上游确认', 'warn'], unknown: ['待查询确认', 'warn'],
  completed: ['已完成', 'ok'], failed: ['失败', 'err'],
};
type FieldValue = string | number | null | undefined | TemplateResult;

class SpOrderDetailDialog extends LitElement {
  static override properties = {
    orderNo: { attribute: false }, order: { state: true }, loading: { state: true },
    errorMessage: { state: true }, copiedValue: { state: true }, copyError: { state: true },
  };
  static override styles = [sharedStyles, modalStyles, orderDetailStyles];

  declare orderNo: string;
  declare private order: AdminOrderDetail | null;
  declare private loading: boolean;
  declare private errorMessage: string;
  declare private copiedValue: string;
  declare private copyError: string;
  private request: AbortSignal | null = null;
  private closed = false;
  private readonly requests = new RequestController(this, () => { if (!this.closed) void this.load(); });

  constructor() {
    super();
    this.orderNo = '';
    this.order = null;
    this.loading = false;
    this.errorMessage = '';
    this.copiedValue = '';
    this.copyError = '';
  }

  protected override willUpdate(changed: PropertyValues<this>) {
    if (!changed.has('orderNo')) return;
    this.requests.abort('detail');
    this.request = null;
    this.closed = false;
    this.order = null;
    this.loading = false;
    this.errorMessage = '';
    this.copiedValue = '';
    this.copyError = '';
    if (this.orderNo && this.isConnected) void this.load();
  }

  protected override updated(changed: PropertyValues<this>) {
    if (!changed.has('orderNo') || !this.orderNo || !this.isConnected) return;
    const dialog = this.renderRoot.querySelector<HTMLDialogElement>('dialog');
    if (!dialog) return;
    if (!dialog.open) dialog.showModal();
    this.renderRoot.querySelector<HTMLButtonElement>('.close-button')?.focus();
  }

  override disconnectedCallback() {
    this.request = null;
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    super.disconnectedCallback();
  }

  private finish(event?: Event) {
    event?.preventDefault();
    if (this.closed || !this.isConnected) return;
    this.closed = true;
    this.requests.abort('detail');
    this.request = null;
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    this.dispatchEvent(new CustomEvent('order-detail-close', { bubbles: true, composed: true }));
  }

  private backdrop(event: MouseEvent) {
    const dialog = event.currentTarget as HTMLDialogElement;
    if (event.target !== dialog) return;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) this.finish();
  }

  private nativeClose(event: Event) {
    // A queued close event must not dismiss a dialog reopened for another order.
    if (!(event.currentTarget as HTMLDialogElement).open) this.finish();
  }

  private async load() {
    const orderNo = this.orderNo;
    if (!orderNo || !this.isConnected || this.closed) return;
    const request = this.requests.next('detail');
    this.request = request;
    this.loading = true;
    this.errorMessage = '';
    try {
      const order = await api.get<AdminOrderDetail>('/admin/transactions/' + encodeURIComponent(orderNo), { signal: request });
      if (!request.aborted && this.request === request && this.isConnected && this.orderNo === orderNo && !this.closed) this.order = order;
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.request === request && this.isConnected && !this.closed) {
        this.errorMessage = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (this.request === request) { this.request = null; this.loading = false; }
      this.requests.release('detail', request);
    }
  }

  private async copy(value: string) {
    const orderNo = this.orderNo;
    try {
      await navigator.clipboard.writeText(value);
      if (this.isConnected && !this.closed && this.orderNo === orderNo) { this.copiedValue = value; this.copyError = ''; }
    } catch {
      if (this.isConnected && !this.closed && this.orderNo === orderNo) {
        this.copiedValue = '';
        this.copyError = '复制失败，请选择文本手动复制。';
      }
    }
  }

  private field(label: string, value: FieldValue, options: { full?: boolean; mono?: boolean; copy?: boolean; missing?: string } = {}) {
    const missing = value === null || value === undefined || value === '';
    return html`<div class=${`detail-field${options.full ? ' full' : ''}`}>
      <dt>${label}</dt><dd class="field-value">
        <span class=${missing ? 'missing' : options.mono ? 'mono' : ''}>${missing ? options.missing ?? '未记录' : value}</span>
        ${options.copy && !missing && typeof value === 'string' ? html`<button class="copy-button" type="button"
          aria-label=${`复制${label}`} @click=${() => void this.copy(value)}>${this.copiedValue === value ? '已复制' : '复制'}</button>` : nothing}
      </dd></div>`;
  }

  private time(value: string | null | undefined) {
    return value ? html`<time datetime=${value}>${dateTime(value)}</time>` : undefined;
  }

  private status(order: AdminOrderDetail) {
    const [label, kind] = order.rawStatus === -1 ? ['已过期', ''] : STATUS_LABEL[order.status] ?? [order.status, ''];
    return html`<span class=${`badge ${kind}`} data-detail-status>${label}</span>`;
  }

  private source(order: AdminOrderDetail) {
    return html`<section class="panel detail-section" aria-labelledby="order-source-title">
      <h2 id="order-source-title">订单与接入来源</h2>
      <dl class="detail-fields">
        ${this.field('商品名称', order.productName, { full: true })}
        ${this.field('外部订单号', order.externalOrderNo, { full: true, mono: true, copy: true })}
        ${this.field('所属分组', order.groupName || (order.groupId !== undefined ? `分组 #${order.groupId}` : undefined))}
        ${this.field('下单凭证', order.apiKeyId !== undefined ? order.apiKeyName || `Key #${order.apiKeyId}` : '无 Key 下单')}
        ${this.field('分组 ID', order.groupId, { mono: true })}
        ${this.field('Key ID', order.apiKeyId, { mono: true })}
        ${this.field('支付渠道', CHANNEL_LABEL[order.payMethod] || order.payMethod)}
        ${this.field('上游订单号', order.providerOrderId, { full: true, mono: true, copy: true })}
        ${this.field('渠道交易号', order.thirdPartyTradeNo, { full: true, mono: true, copy: true })}
      </dl>
    </section>`;
  }

  private lifecycle(order: AdminOrderDetail) {
    const milestones = [
      { label: '创建订单', datetime: order.createdAt },
      ...(order.payAt ? [{ label: '支付到账', datetime: order.payAt }] : []),
      ...(order.refundAt ? [{ label: '最近退款入账', datetime: order.refundAt }] : []),
      ...(order.rawStatus === -1 ? [{ label: '订单到期', datetime: order.expireAt }] : []),
    ].sort((left, right) => Date.parse(left.datetime) - Date.parse(right.datetime));
    return html`<section class="panel detail-section" aria-labelledby="order-time-title">
      <h2 id="order-time-title">订单时间</h2>
      <ol class="timeline">${milestones.map(({ label, datetime }) => html`<li><span class="timeline-title">${label}</span>${this.time(datetime)}</li>`)}</ol>
      <dl class="detail-fields">
        ${this.field('到期时间', this.time(order.expireAt))}
        ${this.field('最后更新', this.time(order.updatedAt))}
        ${this.field('支付时间', this.time(order.payAt))}
        ${this.field('最近退款时间', this.time(order.refundAt))}
      </dl>
      ${order.rawStatus === -1 ? html`<p class="section-note">订单已到期，未记录支付到账。</p>` : nothing}
    </section>`;
  }

  private requestSource(order: AdminOrderDetail) {
    const location = order.clientLocation;
    const local = location?.network && !['public', 'unknown'].includes(location.network);
    const missing = !order.clientIp ? '未记录' : local ? '不适用' : '未解析';
    return html`<section class="panel detail-section request-section" aria-labelledby="order-request-title">
      <h2 id="order-request-title">请求来源</h2>
      <dl class="detail-fields">
        ${this.field('下单来源 IP', order.clientIp, { full: true, mono: true, copy: true })}
        ${this.field('IP 归属地', locationSummary(order.clientIp, location), { full: true })}
        ${this.field('国家 / 地区', cleanLocationValue(location?.country), { missing })}
        ${this.field('省份 / 区域', cleanLocationValue(location?.region), { missing })}
        ${this.field('城市', cleanLocationValue(location?.city), { missing })}
        ${this.field('运营商 / ISP', cleanLocationValue(location?.isp), { missing })}
        ${this.field('网络类型', order.clientIp ? networkLabel(location?.network) : undefined)}
      </dl>
      <p class="section-note">记录的是下单请求来源；API 下单时通常为接入方服务器地址。</p>
    </section>`;
  }

  private financials(order: AdminOrderDetail) {
    const received = Boolean(order.payAt) || ['SUCCESS', 'REFUNDING', 'REFUNDED'].includes(order.status);
    let foreignAmount: string | undefined;
    if (order.payMethod === 'paypal' && order.foreignAmount !== undefined && order.foreignCurrency) {
      try {
        foreignAmount = new Intl.NumberFormat('zh-CN', { style: 'currency', currency: order.foreignCurrency }).format(order.foreignAmount);
      } catch { foreignAmount = `${order.foreignAmount.toFixed(2)} ${order.foreignCurrency}`; }
    }
    const amounts = [
      ['订单金额', yuan(order.amount)], ['累计退款', yuan(order.refundedAmount)],
      ['手续费', received ? yuan(order.fee) : '未入账'], ['结算金额', received ? yuan(order.settleAmount) : '未入账'],
    ];
    return html`<section class="panel detail-section" aria-labelledby="order-money-title">
      <h2 id="order-money-title">金额与结算</h2>
      <dl class="money-grid">${amounts.map(([label, value]) => html`<div class="money-card"><dt>${label}</dt><dd>${value}</dd></div>`)}</dl>
      <div class="financial-meta">
        <span>渠道成本 <strong>${received ? yuan(order.channelCost) : '未入账'}</strong></span>
        ${received ? html`<span>剩余可退 <strong>${yuan(Math.max(0, order.amount - order.refundedAmount))}</strong></span>` : nothing}
        ${foreignAmount ? html`<span>渠道外币金额 <strong>${foreignAmount}</strong></span>` : nothing}
        ${order.payMethod === 'paypal' && order.exchangeRate !== undefined ? html`<span>下单汇率 <strong>${order.exchangeRate}</strong></span>` : nothing}
      </div>
      ${received ? html`<p class="section-note">结算金额为订单到账时扣除手续费后的记录值；退款金额单独累计。</p>` : nothing}
      <dl class="detail-fields">${this.field('最近退款交易号', order.refundTradeNo, { full: true, mono: true, copy: true })}</dl>
    </section>`;
  }

  private refundRecord(refund: AdminRefundOperation) {
    const [label, kind] = REFUND_LABEL[refund.status] ?? [refund.status, ''];
    return html`<article class="record">
      <div class="record-head"><strong>${yuan(refund.amount)}</strong><span class=${`badge ${kind}`}>${label}</span></div>
      <dl class="detail-fields">
        ${this.field('退款请求号', refund.requestId, { full: true, mono: true, copy: true })}
        ${this.field('退款原因', refund.reason, { full: true })}
        ${this.field('渠道退款号', refund.tradeNo, { full: true, mono: true, copy: true })}
        ${this.field('提交时间', this.time(refund.createdAt))}
        ${this.field('最后更新', this.time(refund.updatedAt))}
      </dl>
    </article>`;
  }

  private notificationRecord(notification: AdminOrderNotification) {
    const [label, kind] = NOTIFY_LABEL[notification.status] ?? [String(notification.status), ''];
    let payload = notification.body;
    try { payload = JSON.stringify(JSON.parse(payload), null, 2); } catch { /* Keep a non-JSON recorded payload readable. */ }
    return html`<article class="record">
      <div class="record-head"><span>通知 #${notification.id}</span><span class=${`badge ${kind}`}>${label}</span></div>
      <dl class="detail-fields">
        ${this.field('回调地址', notification.url, { full: true, mono: true, copy: true })}
        ${this.field('重试次数', notification.retryCount)}
        ${this.field('入队时间', this.time(notification.createdAt))}
        ${this.field('最近尝试', this.time(notification.lastAttemptAt), { full: true })}
      </dl>
      ${notification.lastError ? html`<p class="record-error">${notification.lastError}</p>` : nothing}
      <details><summary>查看通知内容</summary><pre>${payload || '未记录通知内容'}</pre></details>
    </article>`;
  }

  private callbacks(order: AdminOrderDetail) {
    return html`<section class="panel detail-section" aria-labelledby="order-callback-title">
      <div class="section-heading"><h2 id="order-callback-title">回调与通知</h2><span class="section-count">${order.notifications.length} 条通知</span></div>
      <dl class="detail-fields">
        ${this.field('支付通知地址', order.notifyUrl, { full: true, mono: true, copy: true, missing: '未配置' })}
        ${this.field('支付后返回地址', order.returnUrl, { full: true, mono: true, copy: true, missing: '未配置' })}
      </dl>
      ${order.notifications.length ? html`<div class="record-list">${repeat(order.notifications, (notification) => notification.id, (notification) => this.notificationRecord(notification))}</div>`
        : html`<p class="empty-records">${order.notifyUrl ? '暂未产生通知记录' : '此订单未配置回调通知'}</p>`}
    </section>`;
  }

  protected override render() {
    if (!this.orderNo) return nothing;
    const order = this.order;
    const canPay = order?.rawStatus === 0 && Date.parse(order.expireAt) > Date.now();
    return html`<dialog class="modal" aria-labelledby="order-detail-title" @cancel=${this.finish} @close=${this.nativeClose} @click=${this.backdrop}>
      <header class="dialog-header">
        <div class="heading"><span class="eyebrow">订单详情</span><h3 id="order-detail-title">${order?.productName || '支付订单'}</h3>
          <div class="heading-meta"><span class="heading-identity"><span class="mono" data-detail-order>${this.orderNo}</span>
            <button class="copy-button" type="button" aria-label="复制订单号" @click=${() => void this.copy(this.orderNo)}>${this.copiedValue === this.orderNo ? '已复制' : '复制'}</button>
          </span>${order ? this.status(order) : nothing}</div>
          ${this.copyError ? html`<p class="err-text small" role="status">${this.copyError}</p>` : nothing}
        </div>
        <div class="header-actions"><button class="refresh-button" type="button" ?disabled=${this.loading} @click=${() => void this.load()}>${this.loading ? '加载中…' : '刷新'}</button>
          <button class="close-button" type="button" aria-label="关闭订单详情" @click=${this.finish}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18"/></svg></button>
        </div>
      </header>
      <div class="detail-body" aria-busy=${this.loading ? 'true' : 'false'}>
        ${this.errorMessage ? order ? html`<p class="refresh-error" role="alert">刷新失败：${this.errorMessage}</p>`
          : html`<div class="status-state" role="alert"><h2>订单详情加载失败</h2><p>${this.errorMessage}</p>
            <button class="primary" type="button" ?disabled=${this.loading} @click=${() => void this.load()}>重新加载</button></div>` : nothing}
        ${order ? html`${this.financials(order)}<div class="detail-grid">${this.source(order)}${this.lifecycle(order)}</div>
          ${this.requestSource(order)}
          <div class="detail-grid">${this.callbacks(order)}<section class="panel detail-section" aria-labelledby="order-refund-title">
            <div class="section-heading"><h2 id="order-refund-title">退款记录</h2><span class="section-count">${order.refunds.length} 笔</span></div>
            ${order.refunds.length ? html`<div class="record-list">${repeat(order.refunds, (refund) => refund.id, (refund) => this.refundRecord(refund))}</div>` : html`<p class="empty-records">此订单暂无退款记录</p>`}
          </section></div>` : this.loading ? html`<div class="status-state" role="status"><h2>正在加载订单详情…</h2><p>读取订单、退款和回调通知记录</p></div>` : nothing}
      </div>
      <footer class="detail-footer"><p>${order ? '金额单位：人民币元 · 时间按当前时区显示' : '订单详情'}</p><div class="footer-links">
        ${canPay ? html`<a href=${`/cashier?orderNo=${encodeURIComponent(this.orderNo)}`} target="_blank" rel="noopener">打开收银台 ↗</a>` : nothing}
        ${order?.notifications.length ? html`<a href=${`/notifications?tab=callbacks&orderNo=${encodeURIComponent(this.orderNo)}`}>查看回调投递</a>` : nothing}
        <button type="button" @click=${this.finish}>关闭</button>
      </div></footer>
    </dialog>`;
  }
}

customElements.define('sp-order-detail-dialog', SpOrderDetailDialog);
