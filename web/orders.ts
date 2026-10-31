import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { api, yuan, percent, count, dateTime, qs, fail, ok, isAbort, requireAuth, confirmDialog, STATUS_LABEL } from './app';
import type { SpSelect, SpDatePicker } from './app';
import type { AdminStats, AdminTransaction, AdminTxPage, GroupWithKeys, RefundResult } from './api-types';
import { sharedStyles, pageStyles } from './styles/shared';
import { ordersStyles } from './styles/orders';
import { RequestController } from './controllers/requests';
import './components/refund-dialog';
import './components/order-detail-dialog';
import { locationSummary } from './services/location';
import { validateForm } from './services/form';

const PAGE_SIZE = 20;
const CHANNEL_LABEL: Readonly<Record<string, string>> = {
  ALIPAY: '支付宝', PAYPAL: 'PayPal', NATIVE: '自有', WECHAT: '微信', USDT: 'USDT',
};
const WALLET_USER = 'demo_buyer@sandbox-pay.local';
const WALLET_PASS = '123456';
const refundable = (order: AdminTransaction) => Number((order.amount - order.refundedAmount).toFixed(2));
type Filters = { keyword: string; groupId: string; status: string; payMethod: string; dateFrom: string; dateTo: string };
const emptyFilters = (): Filters => ({ keyword: '', groupId: '', status: '', payMethod: '', dateFrom: '', dateTo: '' });
function initialFilters(): Filters {
  const params = new URLSearchParams(location.search);
  const status = params.get('status') ?? '';
  const payMethod = params.get('payMethod') ?? '';
  const date = (key: string) => {
    const value = params.get(key) ?? '';
    return /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : '';
  };
  return {
    keyword: params.get('keyword') ?? '',
    groupId: /^\d+$/.test(params.get('groupId') ?? '') ? params.get('groupId')! : '',
    status: ['-1', '0', '1', '2', '3', '4'].includes(status) ? status : '',
    payMethod: ['alipay', 'paypal', 'native'].includes(payMethod) ? payMethod : '',
    dateFrom: date('dateFrom'), dateTo: date('dateTo'),
  };
}
function initialUTCOffset() {
  const raw = new URLSearchParams(location.search).get('utcOffset');
  const value = raw?.trim() ? Number(raw) : NaN;
  return Number.isInteger(value) && value >= -720 && value <= 840 ? value : -new Date().getTimezoneOffset();
}
type Confirmation = { title: string; message: string; confirmText: string; danger?: boolean };

class SpOrdersPage extends LitElement {
  static override properties = {
    authorized: { state: true }, filters: { state: true }, stats: { state: true },
    items: { state: true }, page: { state: true }, pageSize: { state: true }, total: { state: true },
    loading: { state: true }, errorMessage: { state: true }, busy: { state: true }, refundOrder: { state: true },
    groups: { state: true }, groupError: { state: true }, detailOrderNo: { state: true },
  };
  static override styles = [sharedStyles, pageStyles, ordersStyles];

  declare private authorized: boolean;
  declare private filters: Filters;
  declare private stats: AdminStats | null;
  declare private items: AdminTransaction[];
  declare private page: number;
  declare private pageSize: number;
  declare private total: number;
  declare private loading: boolean;
  declare private errorMessage: string;
  declare private busy: ReadonlySet<string>;
  declare private refundOrder: AdminTransaction | null;
  declare private groups: GroupWithKeys[];
  declare private groupError: string;
  declare private detailOrderNo: string;
  private listRequest: AbortSignal | null = null;
  private statsRequest: AbortSignal | null = null;
  private readonly utcOffset = initialUTCOffset();
  private readonly actions = new Map<string, AbortSignal>();
  private readonly requests = new RequestController(this, () => this.reload());

  constructor() {
    super();
    this.authorized = false;
    this.filters = initialFilters();
    this.stats = null;
    this.items = [];
    this.page = 1;
    this.pageSize = PAGE_SIZE;
    this.total = 0;
    this.loading = false;
    this.errorMessage = '';
    this.busy = new Set();
    this.refundOrder = null;
    this.groups = [];
    this.groupError = '';
    this.detailOrderNo = new URLSearchParams(location.search).get('orderNo') ?? '';
  }

  override connectedCallback() {
    super.connectedCallback();
    this.authorized = requireAuth();
    if (this.authorized) {
      this.reload();
      void this.loadGroups();
    }
  }

  override disconnectedCallback() {
    this.listRequest = null;
    this.statsRequest = null;
    this.actions.clear();
    this.busy = new Set();
    this.loading = false;
    this.refundOrder = null;
    super.disconnectedCallback();
  }

  private reload() {
    void this.load(this.page);
    void this.loadStats();
  }

  private async loadGroups() {
    const request = this.requests.next('groups');
    this.groupError = '';
    try {
      const groups = await api.get<GroupWithKeys[]>('/admin/groups', { signal: request });
      if (!request.aborted && this.isConnected) this.groups = groups;
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.isConnected) this.groupError = '分组筛选加载失败';
    } finally {
      this.requests.release('groups', request);
    }
  }

  private showDetails(orderNo: string) {
    this.detailOrderNo = orderNo;
    const url = new URL(location.href);
    if (orderNo) url.searchParams.set('orderNo', orderNo);
    else url.searchParams.delete('orderNo');
    history.replaceState(history.state, '', url);
  }

  private async loadStats() {
    if (!this.isConnected || !this.authorized) return;
    const request = this.requests.next('stats');
    this.statsRequest = request;
    try {
      const stats = await api.get<AdminStats>('/admin/stats', { signal: request });
      if (!request.aborted && this.statsRequest === request && this.isConnected) this.stats = stats;
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.isConnected && this.statsRequest === request) fail(error);
    } finally {
      if (this.statsRequest === request) this.statsRequest = null;
      this.requests.release('stats', request);
    }
  }

  private async load(page: number) {
    if (!this.isConnected || !this.authorized) return;
    const request = this.requests.next('list');
    this.listRequest = request;
    this.page = Math.max(1, Math.floor(page));
    this.loading = true;
    this.errorMessage = '';
    try {
      const result = await api.get<AdminTxPage>('/admin/transactions' + qs({
        ...this.filters, keyword: this.filters.keyword.trim(), page: this.page, pageSize: PAGE_SIZE,
        utcOffset: this.utcOffset,
      }), { signal: request });
      if (request.aborted || this.listRequest !== request || !this.isConnected) return;
      this.items = result.items;
      this.page = result.page;
      this.pageSize = result.pageSize;
      this.total = result.total;
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.isConnected && this.listRequest === request) {
        this.errorMessage = error instanceof Error ? error.message : String(error);
        fail(error);
      }
    } finally {
      if (this.listRequest === request) {
        this.listRequest = null;
        this.loading = false;
      }
      this.requests.release('list', request);
    }
  }

  private editFilter(key: keyof Filters, event: Event) {
    this.filters = { ...this.filters, [key]: (event.currentTarget as HTMLInputElement | SpSelect | SpDatePicker).value };
  }

  private submitFilters(event: SubmitEvent) {
    event.preventDefault();
    const error = validateForm(event.currentTarget as HTMLFormElement);
    if (error) { fail(error); return; }
    void this.load(1);
  }

  private resetFilters(event: Event) {
    event.preventDefault();
    this.filters = emptyFilters();
    void this.load(1);
  }

  private changePage(event: CustomEvent<{ page: number }>) {
    const page = event.detail.page;
    if (Number.isInteger(page) && page >= 1) void this.load(page);
  }

  private setBusy(orderNo: string, pending: boolean) {
    const busy = new Set(this.busy);
    if (pending) busy.add(orderNo);
    else busy.delete(orderNo);
    this.busy = busy;
  }

  private async act(order: AdminTransaction, action: (signal: AbortSignal) => Promise<string>, confirmation?: Confirmation) {
    if (!this.isConnected || this.busy.has(order.orderNo)) return;
    const channel = 'action:' + order.orderNo;
    const request = this.requests.next(channel);
    this.actions.set(order.orderNo, request);
    this.setBusy(order.orderNo, true);
    try {
      if (confirmation && !await confirmDialog(confirmation)) return;
      if (request.aborted || !this.isConnected) return;
      const message = await action(request);
      if (request.aborted || !this.isConnected) return;
      ok(message);
      this.reload();
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.isConnected) fail(error);
    } finally {
      if (this.actions.get(order.orderNo) === request) {
        this.actions.delete(order.orderNo);
        this.setBusy(order.orderNo, false);
      }
      this.requests.release(channel, request);
    }
  }

  private checkRefund(order: AdminTransaction) {
    const amount = order.pendingRefundAmount;
    const requestId = order.refundRequestId;
    if (amount === undefined || amount <= 0 || !requestId) return;
    void this.act(order, async (signal) => {
      const result = await api.post<RefundResult>('/admin/refund', { orderNo: order.orderNo, amount, requestId }, { signal });
      return result.status === 'pending' ? '退款仍在处理中' : '退款已完成';
    });
  }

  private confirmPay(order: AdminTransaction) {
    void this.act(order, async (signal) => {
      await api.post('/native-pay/confirm', {
        orderNo: order.orderNo, walletUser: WALLET_USER, walletPass: WALLET_PASS,
      }, { signal });
      return '已确认收款';
    }, {
      title: '确认收款',
      message: `将 ${order.orderNo}（${yuan(order.amount)}）按存管到账核销，金额以订单原值入账。`,
      confirmText: '确认到账',
    });
  }

  private deleteOrder(order: AdminTransaction) {
    if (order.status === 'REFUNDING') return;
    void this.act(order, async (signal) => {
      await api.del('/admin/transactions/' + encodeURIComponent(order.orderNo), { signal });
      return '已删除';
    }, { title: '删除订单', message: `删除 ${order.orderNo} 后不可恢复。`, confirmText: '删除', danger: true });
  }

  private refund(order: AdminTransaction) {
    if (!this.busy.has(order.orderNo) && order.status === 'SUCCESS' && refundable(order) > 0.001) this.refundOrder = order;
  }

  private refundComplete(event: CustomEvent<RefundResult>) {
    this.refundOrder = null;
    ok(event.detail.status === 'pending' ? '退款处理中，待上游确认' : '退款已完成');
    this.reload();
  }

  private orderRow(order: AdminTransaction): TemplateResult {
    const [label, kind] = order.rawStatus === -1 ? ['已过期', ''] : STATUS_LABEL[order.status] ?? [order.status, ''];
    const busy = this.busy.has(order.orderNo);
    const notification = order.notifyStatus === undefined
      ? (order.notifyUrl ? '回调尚未入队' : '未配置回调')
      : ['回调待发送', '回调成功', '回调重试中', '回调重试耗尽'][order.notifyStatus] ?? '回调状态未知';
    const hasPayment = Boolean(order.payAt) || ['SUCCESS', 'REFUNDING', 'REFUNDED'].includes(order.status);
    const location = locationSummary(order.clientIp, order.clientLocation);
    return html`<tr>
      <td data-order><button class="link order-number mono" @click=${() => this.showDetails(order.orderNo)}>${order.orderNo}</button>
        <span class="cell-note mono" title=${order.externalOrderNo ?? ''}>${order.externalOrderNo ? `外部：${order.externalOrderNo}` : '外部单号未填写'}</span></td>
      <td data-source><span>${order.groupName || (order.groupId ? `分组 #${order.groupId}` : '分组未记录')}</span>
        <span class="cell-note">${order.apiKeyId ? `Key：${order.apiKeyName || `#${order.apiKeyId}`}` : '未使用 API Key'}</span>
        <span class="cell-note request-source" title=${order.clientIp ? `下单请求 IP：${order.clientIp}\n${location}` : '下单请求来源未记录'}>
          <span class="request-ip mono" data-client-ip>${order.clientIp || 'IP 未记录'}</span>
          ${order.clientIp ? html`<span class="source-location" data-client-location>${location}</span>` : nothing}
        </span></td>
      <td class="wrapped product-cell" data-product><span>${order.productName || '商品名未记录'}</span>
        <span class="cell-note" data-channel>${CHANNEL_LABEL[order.channel] || order.channel}
          ${order.thirdPartyTradeNo ? html`<span class="trade-number mono" title=${order.thirdPartyTradeNo}> · ${order.thirdPartyTradeNo}</span>` : nothing}</span></td>
      <td class="num" data-amount>${yuan(order.amount)}</td>
      <td class="num" data-refunded>${order.refundedAmount > 0 ? yuan(order.refundedAmount) : '—'}
        ${order.pendingRefundAmount ? html`<span class="cell-note">处理中 ${yuan(order.pendingRefundAmount)}</span>` : nothing}</td>
      <td class="num" data-fee>${hasPayment ? yuan(order.fee) : '—'}
        <span class="cell-note">${hasPayment ? `结算 ${yuan(order.settleAmount)}` : '尚未结算'}</span></td>
      <td><span class=${kind ? `badge ${kind}` : 'badge'} data-status>${label}</span>
        <span class=${`cell-note notify-note ${order.notifyStatus === 2 || order.notifyStatus === 3 ? 'notify-error' : ''}`}
          title=${order.notifyLastError || order.notifyUrl || ''}>${notification}${order.notifyStatus !== undefined && order.notifyRetryCount ? ` · ${order.notifyRetryCount} 次重试` : ''}</span></td>
      <td class="small" data-time><time datetime=${order.createdAt}>${dateTime(order.createdAt)}</time>
        <span class="cell-note">${order.payAt ? `支付 ${dateTime(order.payAt)}` : `到期 ${dateTime(order.expireAt) || '未记录'}`}</span></td>
      <td class="acts" data-acts>
        <button class="link" @click=${() => this.showDetails(order.orderNo)}>详情</button>
        ${order.status === 'PENDING' && order.payMethod === 'native'
          ? html`<button class="link" ?disabled=${busy} @click=${() => this.confirmPay(order)}>确认收款</button>` : nothing}
        ${order.status === 'SUCCESS' && refundable(order) > 0.001
          ? html`<button class="link" ?disabled=${busy} @click=${() => this.refund(order)}>退款</button>` : nothing}
        ${order.status === 'REFUNDING' && order.refundRequestId && order.pendingRefundAmount
          ? html`<button class="link" ?disabled=${busy} @click=${() => this.checkRefund(order)}>${busy ? '查询中…' : '查询退款'}</button>` : nothing}
        ${order.status !== 'REFUNDING'
          ? html`<button class="link danger" ?disabled=${busy} @click=${() => this.deleteOrder(order)}>删除</button>` : nothing}
      </td>
    </tr>`;
  }

  protected override render() {
    if (!this.authorized) return nothing;
    const stats = this.stats;
    const cards = [
      ['成交金额', stats ? yuan(stats.amount) : '—'],
      ['成功笔数', stats ? `${count(stats.successCount)} 笔` : '—'],
      ['成功率', stats && stats.successRate !== null ? percent(stats.successRate) : '—'],
    ];
    return html`
      <main class="wrap">
        <div class="orders-head"><div class="page-head"><h1>订单</h1><p>查看接入来源、支付明细、退款与回调记录</p></div>
          <button type="button" ?disabled=${this.loading} @click=${() => this.reload()}>${this.loading ? '刷新中…' : '刷新订单'}</button></div>
        <div class="stats" id="stats">${cards.map(([label, value]) => html`
          <div class="stat"><div class="k" data-k>${label}</div><div class="v" data-v>${value}</div></div>
        `)}</div>
        <div class="panel">
          <form class="row" id="filters" novalidate @submit=${this.submitFilters} @reset=${this.resetFilters}>
            <div class="field grow">
              <label for="keyword">关键词</label>
              <input id="keyword" placeholder="订单号 / 外部单号 / 商品 / 渠道交易号" .value=${this.filters.keyword}
                @input=${(event: Event) => this.editFilter('keyword', event)}>
            </div>
            <div class="field sm">
              <label for="groupId">接入分组</label>
              <sp-select id="groupId" label="接入分组" .value=${this.filters.groupId}
                .options=${[{ value: '', label: '全部分组' }, ...this.groups.map((group) => ({ value: String(group.id), label: `${group.name}${group.isActive ? '' : '（已停用）'}` })),
                  ...(this.filters.groupId && !this.groups.some((group) => String(group.id) === this.filters.groupId) ? [{ value: this.filters.groupId, label: `分组 #${this.filters.groupId}（历史记录）` }] : [])]}
                @change=${(event: Event) => this.editFilter('groupId', event)}></sp-select>
            </div>
            <div class="field sm">
              <label for="status">状态</label>
              <sp-select id="status" label="订单状态" .value=${this.filters.status}
                .options=${[{ value: '', label: '全部' }, { value: '0', label: '待支付' }, { value: '1', label: '已支付' },
                  { value: '2', label: '失败' }, { value: '3', label: '退款中' }, { value: '4', label: '已退款' }, { value: '-1', label: '已过期' }]}
                @change=${(event: Event) => this.editFilter('status', event)}></sp-select>
            </div>
            <div class="field sm">
              <label for="payMethod">渠道</label>
              <sp-select id="payMethod" label="支付渠道" .value=${this.filters.payMethod}
                .options=${[{ value: '', label: '全部' }, { value: 'alipay', label: '支付宝' }, { value: 'paypal', label: 'PayPal' }, { value: 'native', label: '自有' }]}
                @change=${(event: Event) => this.editFilter('payMethod', event)}></sp-select>
            </div>
            <div class="field sm">
              <label for="dateFrom">起始日期</label>
              <sp-date-picker id="dateFrom" label="起始日期" .value=${this.filters.dateFrom} @input=${(event: Event) => this.editFilter('dateFrom', event)}></sp-date-picker>
            </div>
            <div class="field sm">
              <label for="dateTo">截止日期</label>
              <sp-date-picker id="dateTo" label="截止日期" .value=${this.filters.dateTo} @input=${(event: Event) => this.editFilter('dateTo', event)}></sp-date-picker>
            </div>
            <button class="primary" type="submit">查询</button><button type="reset">重置</button>
          </form>
          ${this.groupError ? html`<p class="err-text small" role="alert">${this.groupError} <button class="link" @click=${() => void this.loadGroups()}>重试</button></p>` : nothing}
        </div>
        <div class="panel" aria-busy=${this.loading ? 'true' : 'false'}>
          <div class="table-scroll"><table>
            <thead><tr><th>订单 / 外部单号</th><th>接入 / 请求来源</th><th>商品 / 渠道</th><th class="num">金额</th>
              <th class="num">已退</th><th class="num">手续费 / 结算</th><th>订单 / 回调状态</th><th>创建 / 支付时间</th><th>操作</th></tr></thead>
            <tbody id="rows">${repeat(this.items, (order) => order.orderNo, (order) => this.orderRow(order))}</tbody>
          </table></div>
          ${this.errorMessage ? html`<p class="err-text small" role="alert">${this.errorMessage}</p>` : nothing}
          ${!this.items.length ? html`<div class="empty" id="empty" role="status">
            ${this.loading ? '正在加载订单…' : this.errorMessage ? '订单加载失败' : '没有符合条件的订单'}
          </div>` : nothing}
          <sp-pager .page=${this.page} .pageSize=${this.pageSize} .total=${this.total} @page-change=${this.changePage}></sp-pager>
        </div>
      </main>
      ${this.refundOrder ? html`<sp-refund-dialog .order=${this.refundOrder}
        @refund-close=${() => { this.refundOrder = null; }} @refund-complete=${this.refundComplete}></sp-refund-dialog>` : nothing}
      ${this.detailOrderNo ? html`<sp-order-detail-dialog .orderNo=${this.detailOrderNo}
        @order-detail-close=${() => this.showDetails('')}></sp-order-detail-dialog>` : nothing}`;
  }
}

customElements.define('sp-orders-page', SpOrdersPage);
