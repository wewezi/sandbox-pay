import { LitElement, html, nothing } from 'lit';
import { keyed } from 'lit/directives/keyed.js';
import {
  api, mustBe, isAbort, ok, fail, toast, yuan, dateTime, mmss,
} from './app';
import type { CashierInfo, CashierPending, CashierPaid, CashierClosed, GatewayPayResult } from './api-types';
import { sharedStyles } from './styles/shared';
import { cashierStyles } from './styles/cashier';
import { navigate } from './services/router';

const pollMs = 5000;
const tickMs = 1000;
const queryTimeoutMs = 12000;
const walletUser = 'demo_buyer@sandbox-pay.local';
const usdFormat = new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: false });
const rateFormat = new Intl.NumberFormat('en-US', { maximumFractionDigits: 8, useGrouping: false });
type Channel = 'alipay' | 'paypal';

const iconPaths = {
  arrow: 'M5 12h14m-5-5 5 5-5 5',
  chevron: 'm9 5 7 7-7 7',
  timer: 'M12 8v4l3 2M9 2h6M12 5a8 8 0 1 0 0 16 8 8 0 0 0 0-16Z',
  check: 'm7 12 3 3 7-7',
  clock: 'M12 7v5l3 2M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z',
  alert: 'M12 8v5m0 3h.01M10.3 4.6 2.5 18a2 2 0 0 0 1.7 3h15.6a2 2 0 0 0 1.7-3L13.7 4.6a2 2 0 0 0-3.4 0Z',
  info: 'M12 11v5m0-8h.01M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z',
  store: 'M4 10v10h16V10M3 10l2-6h14l2 6M3 10a3 3 0 0 0 6 0 3 3 0 0 0 6 0 3 3 0 0 0 6 0M9 20v-6h6v6',
} as const;

function icon(name: keyof typeof iconPaths) {
  return html`<svg class="icon-svg" viewBox="0 0 24 24" aria-hidden="true"><path d=${iconPaths[name]} pathLength="1"/></svg>`;
}

function actionIcon(busy: boolean) {
  return busy ? html`<span class="action-spinner" aria-hidden="true"></span>` : icon('arrow');
}

function channelName(method: string) {
  if (method === 'alipay') return '支付宝';
  if (method === 'paypal') return 'PayPal';
  return 'Mintweft';
}

function dollars(amount: number) {
  return `USD ${usdFormat.format(amount)}`;
}

function rateLabel(rate: number) {
  return `1 CNY = ${rateFormat.format(rate)} USD`;
}

function kv(key: string, value: string | number | null | undefined, mono = true) {
  if (value === null || value === undefined || value === '') return nothing;
  return html`<div class="kv"><span class="k" data-k>${key}</span><span class=${mono ? 'v mono' : 'v'} data-v>${value}</span></div>`;
}

/** Lit updates preserve native input values and open details during polling. */
class SpCashier extends LitElement {
  static override styles = [sharedStyles, cashierStyles];
  static override properties = {
    info: { state: true }, errorMessage: { state: true }, countdown: { state: true },
    connectingChannel: { state: true }, paying: { state: true },
    connectionMessage: { state: true }, refreshing: { state: true }, walletSubmitted: { state: true },
    channelError: { state: true }, failedChannel: { state: true },
  };
  declare info: CashierInfo | null;
  declare errorMessage: string;
  declare countdown: string;
  declare connectingChannel: Channel | null;
  declare paying: boolean;
  declare connectionMessage: string;
  declare refreshing: boolean;
  declare walletSubmitted: boolean;
  declare channelError: string;
  declare failedChannel: Channel | null;

  #lifecycle: AbortController | null = null;
  #query: AbortController | null = null;
  #generation = 0;
  #queryVersion = 0;
  #operation = 0;
  #orderNo = '';
  #poll: number | null = null;
  #tick: number | null = null;
  #expiredDeadline: string | null = null;
  #pinLength = 0;
  #pinIndex = 0;
  #pinSelectionStart = 0;
  #pinSelectionEnd = 0;
  #walletUserValid = true;
  #qrSource = '';
  #qrState: 'loading' | 'loaded' | 'error' = 'loading';
  #qrAttempt = 0;
  #navigating = false;
  #onPageHide = () => this.#stopSession();
  #onOnline = () => {
    if (!this.#busy && (this.connectionMessage || this.errorMessage)) void this.#fetchInfo(true);
  };
  #onPageShow = (event: PageTransitionEvent) => {
    // BFCache restores this element without calling connectedCallback again.
    if (event.persisted && this.isConnected) this.#startSession();
  };

  constructor() {
    super();
    this.info = null;
    this.errorMessage = '';
    this.countdown = '';
    this.connectingChannel = null;
    this.paying = false;
    this.connectionMessage = '';
    this.refreshing = false;
    this.walletSubmitted = false;
    this.channelError = '';
    this.failedChannel = null;
  }

  override connectedCallback() {
    super.connectedCallback();
    window.addEventListener('pagehide', this.#onPageHide);
    window.addEventListener('pageshow', this.#onPageShow);
    window.addEventListener('online', this.#onOnline);
    this.#startSession();
  }

  #startSession() {
    this.#stopSession();
    this.#lifecycle = new AbortController();
    this.#orderNo = new URLSearchParams(location.search).get('orderNo') || '';
    this.#expiredDeadline = null;
    this.#navigating = false;
    this.info = null;
    this.errorMessage = '';
    this.countdown = '';
    this.connectingChannel = null;
    this.paying = false;
    this.connectionMessage = '';
    this.refreshing = false;
    this.walletSubmitted = false;
    this.channelError = '';
    this.failedChannel = null;
    this.#walletUserValid = true;
    this.#qrSource = '';
    this.#qrState = 'loading';
    this.#qrAttempt = 0;
    this.#resetPin();
    if (!this.#orderNo) {
      this.errorMessage = '支付链接缺少订单号，请重新获取支付链接';
      return;
    }
    this.#consumeReturnStatus();
    void this.#fetchInfo(true);
  }

  override disconnectedCallback() {
    window.removeEventListener('pagehide', this.#onPageHide);
    window.removeEventListener('pageshow', this.#onPageShow);
    window.removeEventListener('online', this.#onOnline);
    this.#stopSession();
    super.disconnectedCallback();
  }

  #stopSession() {
    this.#generation++;
    this.#operation++;
    this.#lifecycle?.abort();
    this.#lifecycle = null;
    this.#cancelQuery();
    this.#stopTimers();
    this.#resetPin();
    const pin = this.renderRoot.querySelector<HTMLInputElement>('[data-wallet-pass]');
    if (pin) pin.value = '';
  }

  get #busy() {
    return this.connectingChannel !== null || this.paying || this.#navigating;
  }

  #current(generation: number) {
    return this.isConnected && generation === this.#generation && this.#lifecycle !== null && !this.#lifecycle.signal.aborted;
  }

  #cancelQuery() {
    this.#queryVersion++;
    this.#query?.abort();
    this.#query = null;
    this.refreshing = false;
  }

  #stopPoll() {
    if (this.#poll !== null) window.clearInterval(this.#poll);
    this.#poll = null;
  }

  #stopTick() {
    if (this.#tick !== null) window.clearInterval(this.#tick);
    this.#tick = null;
  }

  #stopTimers() {
    this.#stopPoll();
    this.#stopTick();
  }

  #consumeReturnStatus() {
    const params = new URLSearchParams(location.search);
    const status = params.get('status');
    if (!status) return;
    if (status === 'cancelled') toast('已取消 PayPal 支付');
    else if (status === 'failed') fail('PayPal 支付未完成');
    params.delete('status');
    const rest = params.toString();
    history.replaceState(null, '', location.pathname + (rest ? '?' + rest : '') + location.hash);
  }

  async #fetchInfo(force = false) {
    const lifecycle = this.#lifecycle;
    if (!lifecycle || lifecycle.signal.aborted || !this.isConnected || !this.#orderNo) return;
    if (this.#query && !force) return;
    if (force) this.#cancelQuery();
    const controller = new AbortController();
    this.#query = controller;
    this.refreshing = true;
    const generation = this.#generation;
    const version = ++this.#queryVersion;
    const timeout = AbortSignal.timeout(queryTimeoutMs);
    const signal = AbortSignal.any([lifecycle.signal, controller.signal, timeout]);
    try {
      const data = await api.get<CashierInfo>(
        `/native-pay/cashier?orderNo=${encodeURIComponent(this.#orderNo)}`, { signal },
      );
      if (!this.#current(generation) || version !== this.#queryVersion || signal.aborted) return;
      if (data.orderNo !== this.#orderNo) throw new Error('订单查询结果与支付链接不匹配');
      if (this.info && this.info.status !== 'pending' && data.status === 'pending') return;
      this.info = data;
      if (data.status !== 'pending' || !this.#availableChannels(data).includes('native')) {
        this.#resetPin();
        this.#walletUserValid = true;
      }
      if (data.status !== 'pending') this.walletSubmitted = false;
      if (data.status === 'pending') {
        const source = this.#qrURL(data);
        if (source !== this.#qrSource) {
          this.#qrSource = source;
          this.#qrState = 'loading';
          this.#qrAttempt = 0;
        }
      }
      this.errorMessage = '';
      this.connectionMessage = '';
      this.#syncTimers();
    } catch (error) {
      if (lifecycle.signal.aborted || controller.signal.aborted || !this.#current(generation) || version !== this.#queryVersion) return;
      if (isAbort(error) && !timeout.aborted) return;
      const message = timeout.aborted ? '订单读取超时，请重新加载' : error instanceof Error ? error.message : '订单查询失败';
      if (this.info) {
        this.connectionMessage = message;
        this.#syncTimers();
      } else {
        this.#stopTimers();
        this.errorMessage = message;
      }
    } finally {
      if (this.#query === controller) {
        this.#query = null;
        this.refreshing = false;
      }
    }
  }

  #syncTimers() {
    if (this.info?.status !== 'pending' || this.errorMessage || !this.isConnected) {
      this.#stopTimers();
      this.countdown = '';
      return;
    }
    this.#updateCountdown();
    if (!this.#busy && this.#poll === null) {
      this.#poll = window.setInterval(() => {
        if (!this.#busy) void this.#fetchInfo();
      }, pollMs);
    }
    const deadline = Date.parse(this.info.expireAt);
    if (Number.isFinite(deadline) && deadline > Date.now() && this.#tick === null) {
      this.#tick = window.setInterval(() => this.#updateCountdown(), tickMs);
    }
  }

  #updateCountdown() {
    const info = this.info;
    if (!info || info.status !== 'pending') return;
    const deadline = Date.parse(info.expireAt);
    if (!Number.isFinite(deadline)) {
      this.countdown = '';
      this.#stopTick();
      return;
    }
    const text = mmss(Math.floor((deadline - Date.now()) / 1000));
    this.countdown = text === null ? '订单已过期' : `支付剩余时间 ${text}`;
    if (text !== null) return;
    this.#stopTick();
    if (!this.#busy && this.#expiredDeadline !== info.expireAt) {
      this.#expiredDeadline = info.expireAt;
      const generation = this.#generation;
      queueMicrotask(() => {
        if (this.#current(generation) && this.info?.status === 'pending' && !this.#busy) void this.#fetchInfo(true);
      });
    }
  }

  #beginOperation() {
    const lifecycle = this.#lifecycle;
    if (!lifecycle || lifecycle.signal.aborted || !this.isConnected || this.#busy || this.errorMessage || this.connectionMessage || this.walletSubmitted ||
      this.info?.status !== 'pending' || this.countdown === '订单已过期') return null;
    this.#stopPoll();
    this.#cancelQuery();
    return { generation: this.#generation, id: ++this.#operation, signal: lifecycle.signal };
  }

  #operationCurrent(operation: { generation: number; id: number }) {
    return this.#current(operation.generation) && operation.id === this.#operation;
  }

  async #gotoGateway(channel: Channel) {
    if (!this.info || this.info.status !== 'pending' || !this.#availableChannels(this.info).includes(channel)) return;
    if (channel === 'paypal' && !this.#paypalAvailable(this.info)) return;
    const operation = this.#beginOperation();
    if (!operation) return;
    this.channelError = '';
    this.failedChannel = null;
    this.connectingChannel = channel;
    try {
      const result = await api.post<GatewayPayResult>('/native-pay/switch-channel',
        { orderNo: this.#orderNo, channel }, { signal: operation.signal });
      if (!this.#operationCurrent(operation) || this.info?.status !== 'pending') return;
      if (!result.data) throw new Error('网关未返回跳转信息');
      const target = new URL(result.data);
      if (target.protocol !== 'https:' && target.protocol !== 'http:') throw new Error('网关返回的支付链接无效');
      this.#navigating = true;
      this.#stopTimers();
      location.assign(target.href);
    } catch (error) {
      if (!isAbort(error) && this.#operationCurrent(operation)) {
        this.#navigating = false;
        this.channelError = error instanceof Error ? error.message : '支付渠道暂时不可用';
        this.failedChannel = channel;
        fail(this.channelError);
      }
    } finally {
      if (this.#operationCurrent(operation)) {
        if (!this.#navigating) {
          // Refresh the server's available channels before allowing another payment.
          await this.#fetchInfo(true);
        }
        if (this.#operationCurrent(operation)) {
          this.connectingChannel = null;
          if (!this.#navigating) this.#syncTimers();
        }
      }
    }
  }

  #resetPin() {
    this.#pinLength = 0;
    this.#pinIndex = 0;
    this.#pinSelectionStart = 0;
    this.#pinSelectionEnd = 0;
  }

  #syncPinPosition(input: HTMLInputElement) {
    const length = input.value.length;
    const index = Math.min(input.selectionStart ?? length, 5);
    const start = input.selectionStart ?? length;
    const end = input.selectionEnd ?? start;
    if (length === this.#pinLength && index === this.#pinIndex && start === this.#pinSelectionStart && end === this.#pinSelectionEnd) return;
    this.#pinLength = length;
    this.#pinIndex = index;
    this.#pinSelectionStart = start;
    this.#pinSelectionEnd = end;
    this.requestUpdate();
  }

  #normalizePin(input: HTMLInputElement) {
    const original = input.value;
    const clean = original.replace(/[^0-9]/g, '').slice(0, 6);
    if (clean !== original) {
      const start = original.slice(0, input.selectionStart ?? original.length).replace(/[^0-9]/g, '').length;
      const end = original.slice(0, input.selectionEnd ?? original.length).replace(/[^0-9]/g, '').length;
      input.value = clean;
      input.setSelectionRange(Math.min(start, clean.length), Math.min(end, clean.length));
    }
    this.#syncPinPosition(input);
  }

  #onPinInput(event: Event) {
    if (event instanceof InputEvent && event.isComposing) return;
    this.#normalizePin(event.currentTarget as HTMLInputElement);
  }

  #onPinBeforeInput(event: InputEvent) {
    if (!event.isComposing && event.inputType === 'insertText' && event.data && /[^0-9]/.test(event.data)) event.preventDefault();
  }

  #onPinPaste(event: ClipboardEvent) {
    const text = event.clipboardData?.getData('text');
    if (text === undefined) return;
    event.preventDefault();
    const input = event.currentTarget as HTMLInputElement;
    const digits = text.replace(/[^0-9]/g, '');
    if (!digits) return;
    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? input.value.length;
    const capacity = 6 - (input.value.length - (end - start));
    input.setRangeText(digits.slice(0, capacity), start, end, 'end');
    this.#normalizePin(input);
  }

  #onPinClick(event: MouseEvent) {
    const input = event.currentTarget as HTMLInputElement;
    if (event.detail !== 1 || event.shiftKey || (input.selectionEnd ?? 0) - (input.selectionStart ?? 0) > 1) {
      this.#syncPinPosition(input);
      return;
    }
    const cells = input.parentElement?.querySelectorAll<HTMLElement>('[data-pin-cell]');
    if (!cells) return;
    const clicked = Array.from(cells).findIndex(cell => event.clientX <= cell.getBoundingClientRect().right);
    const index = Math.min(clicked < 0 ? 5 : clicked, input.value.length);
    input.setSelectionRange(index, Math.min(index + 1, input.value.length));
    this.#syncPinPosition(input);
  }

  async #payWithWallet() {
    if (!this.info || !this.#availableChannels(this.info).includes('native')) return;
    const user = mustBe('[data-wallet-user]', HTMLInputElement, this.renderRoot);
    this.#syncWalletUser(user);
    if (!this.#walletUserValid) {
      user.reportValidity();
      user.focus();
      return;
    }
    const pass = this.renderRoot.querySelector<HTMLInputElement>('[data-wallet-pass]');
    if (!pass) return;
    this.#normalizePin(pass);
    if (!/^[0-9]{6}$/.test(pass.value)) {
      pass.focus();
      fail('请输入 6 位数字支付密码');
      return;
    }
    const operation = this.#beginOperation();
    if (!operation) return;
    this.channelError = '';
    this.failedChannel = null;
    this.paying = true;
    let refreshed = false;
    try {
      // Selecting Mintweft changes the server's current choice before clearing
      // this order. Keep the same account/PIN nodes while that choice refreshes.
      if (this.info?.status === 'pending' && (this.info.payMethod || '').toLowerCase() !== 'native') {
        await api.post<GatewayPayResult>('/native-pay/switch-channel', {
          orderNo: this.#orderNo, channel: 'native',
        }, { signal: operation.signal });
        if (!this.#operationCurrent(operation)) return;
        await this.#fetchInfo(true);
        if (!this.#operationCurrent(operation) || this.info?.status !== 'pending') return;
        if (this.connectionMessage || this.info.payMethod !== 'native') {
          throw new Error('支付方式正在确认，请稍后重试');
        }
      }
      await api.post('/native-pay/sandbox-confirm', {
        orderNo: this.#orderNo, walletUser: user.value.trim(), walletPass: pass.value,
      }, { signal: operation.signal });
      if (!this.#operationCurrent(operation)) return;
      this.walletSubmitted = true;
      toast('付款已提交，正在确认结果');
      await this.#fetchInfo(true);
      refreshed = true;
    } catch (error) {
      if (!isAbort(error) && this.#operationCurrent(operation)) fail(error instanceof Error ? error.message : '支付失败');
    } finally {
      if (this.#operationCurrent(operation)) {
        if (!refreshed) await this.#fetchInfo(true);
        if (this.#operationCurrent(operation)) {
          this.paying = false;
          this.#syncTimers();
        }
      }
    }
  }

  async #copy(value: string) {
    const generation = this.#generation;
    try {
      await navigator.clipboard.writeText(value);
      if (this.#current(generation)) ok('已复制');
    } catch {
      if (this.#current(generation)) fail('复制失败，请手动选中复制');
    }
  }

  #retry() {
    if (!this.#orderNo) return location.reload();
    if (!this.info) this.errorMessage = '';
    void this.#fetchInfo(true);
  }

  #syncWalletUser(input: HTMLInputElement) {
    this.#walletUserValid = input.value.trim().length >= 5;
    input.setCustomValidity(this.#walletUserValid ? '' : '请输入至少 5 个字符的 Mintweft 账号');
    this.requestUpdate();
  }

  #availableChannels(info: CashierInfo): ('native' | Channel)[] {
    if (info.status !== 'pending') return [];
    if (info.availableChannels) return info.availableChannels;
    // Pending orders may change their selected method before payment completes.
    return ['native', 'alipay', 'paypal'];
  }

  #leave() {
    let target = new URL('/sandbox', location.origin);
    try {
      const configured = this.info?.returnUrl?.trim();
      if (configured) {
        const candidate = new URL(configured, location.origin);
        if (candidate.protocol === 'http:' || candidate.protocol === 'https:') target = candidate;
      }
    } catch { /* Use the explicit default when the configured address is invalid. */ }
    navigate(target.href);
  }

  #qrURL(info: CashierPending) {
    return info.paymentInfo?.qrCodeUrl?.trim() ||
      'https://api.qrserver.com/v1/create-qr-code/?size=300x300&margin=8&data=' + encodeURIComponent(`SANDBOX_PAY_ORDER_${info.orderNo}`);
  }

  #channelErrorHint() {
    const provider = channelName(this.failedChannel || '');
    if (/凭证|认证|授权/.test(this.channelError)) return `${provider} 认证未通过，可更换方式。`;
    return `${provider} 暂不可用，可更换方式。`;
  }

  #paypalAvailable(info: CashierPending) {
    const quote = info.paypalQuote;
    if (!quote) return true;
    return quote.available && quote.currency === 'USD' && quote.amount !== null &&
      Number.isFinite(quote.amount) && quote.amount >= 0.01 && quote.exchangeRate !== null &&
      Number.isFinite(quote.exchangeRate) && quote.exchangeRate > 0;
  }

  #paypalDescription(info: CashierPending) {
    const quote = info.paypalQuote;
    if (!quote) return '使用 PayPal 账户付款';
    if (this.#paypalAvailable(info) && quote.exchangeRate !== null) return rateLabel(quote.exchangeRate);
    if (/美分|最低|过小|不足/.test(quote.reason || '')) return '折算不足 0.01 USD，可换其他方式';
    return '美元金额暂不可用，可换其他方式';
  }

  #onQRState(event: Event, state: 'loaded' | 'error') {
    const image = event.currentTarget as HTMLImageElement;
    if (!image.isConnected || image.getAttribute('src') !== this.#qrSource) return;
    this.#qrState = state;
    this.requestUpdate();
  }

  #retryQR() {
    this.#qrState = 'loading';
    this.#qrAttempt++;
    this.requestUpdate();
  }

  protected override render() {
    return html`<div class="cashier-page"><header class="cashier-topbar">
      <div class="brand-group"><div class="cashier-head">Mintweft</div><span class="cashier-label">收银台</span></div>
      <sp-theme-toggle .showLabel=${false}></sp-theme-toggle>
    </header>${this.#renderCheckout(this.info)}
      <footer class="cashier-footer">由 Mintweft 提供支付服务</footer></div>`;
  }

  #renderAmount(amount: number) {
    const formatted = yuan(amount);
    return html`<div class=${formatted.length > 11 ? 'amount amount-long' : 'amount'} data-amount><span class="currency">￥</span><span>${formatted.slice(1)}</span></div>`;
  }

  #renderCheckout(info: CashierInfo | null) {
    const pending = info?.status === 'pending';
    const loading = !info && !this.errorMessage;
    return html`<main class=${loading ? 'checkout-shell checkout-loading' : 'checkout-shell'} aria-label="订单支付" aria-busy=${String(loading)}>
      ${info ? this.#renderOrderSummary(info) : this.#renderLoadingSummary()}
      <section class=${pending || loading ? 'payment-pane' : 'payment-pane result-pane'} aria-labelledby=${pending || loading ? 'payment-title' : 'result-title'}>
        ${info ? pending ? this.#renderPending(info) : info.status === 'paid' ? this.#renderPaid(info) : this.#renderClosed(info)
          : this.errorMessage ? this.#renderError() : this.#renderLoading()}
      </section>
    </main>`;
  }

  #renderLoadingSummary() {
    return html`<section class="order-summary skeleton-summary" aria-hidden="true">
      <div class="summary-heading"><span class="skeleton skeleton-caption"></span><span class="skeleton skeleton-currency"></span></div>
      <div class="skeleton skeleton-product"></div>
      <div class="amount-label"><span class="skeleton skeleton-caption"></span></div>
      <div class="skeleton skeleton-amount"></div>
      <div class="countdown"><span class="skeleton skeleton-dot"></span><span class="skeleton skeleton-time"></span></div>
      <div class="summary-fields"><div class="kv skeleton-row"><span class="skeleton skeleton-caption"></span><span class="skeleton skeleton-value"></span></div></div>
      <div class="order-details skeleton-details"><span class="skeleton skeleton-caption"></span><span class="skeleton skeleton-dot"></span></div>
      <div class="summary-note"><span class="skeleton skeleton-dot"></span><div class="skeleton-note"><span class="skeleton skeleton-text"></span><span class="skeleton skeleton-text-short"></span></div></div>
    </section>`;
  }

  #renderLoading() {
    return html`<div class="payment-heading loading-heading"><h2 id="payment-title">正在加载订单</h2>
      <div class="payment-subtitle" role="status" aria-live="polite"><span class="loading-status-dot" aria-hidden="true"></span><span>正在读取付款信息，请稍候…</span></div>
      </div>
      <div class="payment-methods" aria-hidden="true">${[0, 1, 2].map(() => html`<div class="payment-method skeleton-method">
        <span class="skeleton skeleton-provider"></span><div class="method-copy"><span class="skeleton skeleton-method-title"></span><span class="skeleton skeleton-method-description"></span></div><span class="skeleton skeleton-dot"></span>
      </div>`)}</div>
      <div class="qr-section" aria-hidden="true"><div class="qr-surface skeleton-qr-surface"><div class="qr-frame skeleton skeleton-qr"></div></div>
        <div class="skeleton-qr-copy"><span class="skeleton skeleton-method-title"></span><span class="skeleton skeleton-text"></span><span class="skeleton skeleton-text-short"></span></div>
      </div>`;
  }

  #renderOrderSummary(info: CashierInfo) {
    const pending = info.status === 'pending';
    const paid = info.status === 'paid';
    const pay = pending ? info.paymentInfo ?? {} : {};
    const project = info.projectName?.trim();
    const receiver = pay.accountName?.trim();
    const external = info.externalOrderNo?.trim();
    const orderNumber = external || info.orderNo;
    const orderLabel = external && external !== info.orderNo ? '商户订单号' : '订单号';
    const transfer = pending && ['', 'native', 'bank'].includes((info.payMethod || '').toLowerCase()) &&
      Boolean(pay.accountNo?.trim() || pay.bankName?.trim() || (pay.qrCodeUrl?.trim() && pay.qrCodeChannel === 'native'));
    const remark = transfer ? pay.remark?.trim() : '';
    return html`<section class="order-summary" aria-labelledby="order-title">
        <div class="summary-heading">${project ? html`<span class="order-source" aria-label=${`订单来源：${project}`}>${icon('store')}<span>${project}</span></span>` : html`<span class="eyebrow">订单摘要</span>`}<span class="currency-label">CNY</span></div>
        <h1 class="product" id="order-title" data-product>${info.productName}</h1>
        <p class="amount-label">${paid ? info.payMethod === 'paypal' ? '订单金额' : '支付金额' : '应付金额'}</p>${this.#renderAmount(info.amount)}
        <div class=${paid ? 'countdown summary-paid' : 'countdown'} data-countdown=${pending ? '' : nothing} role=${pending ? 'timer' : nothing}>${icon(pending ? 'timer' : paid ? 'check' : 'clock')}<span>${pending ? this.countdown || '等待付款' : paid ? '付款已确认' : info.status === 'expired' ? '订单已关闭' : '付款未完成'}</span></div>
        <div class="summary-fields">
          ${receiver && receiver !== project ? kv('收款方', receiver, false) : nothing}
          <div class="kv"><span class="k">${orderLabel}</span><span class="v mono"><span data-order-number>${orderNumber}</span>
            <button class="copy-inline" type="button" data-order-copy aria-label=${`复制${orderLabel}`} @click=${() => void this.#copy(orderNumber)}>复制</button></span></div>
          ${info.createdAt ? kv('下单时间', dateTime(info.createdAt), false) : nothing}
        </div>
        ${transfer ? html`<section class="order-details" aria-labelledby="receiver-title"><h2 id="receiver-title">收款信息</h2>
          <div data-details>${kv('收款账号', pay.accountNo)}${kv('开户银行', pay.bankName, false)}</div>
          ${remark ? html`<div class="kv"><span class="k">转账附言</span><span class="v mono"><span data-remark>${remark}</span>
            <button class="copy-inline" type="button" data-copy @click=${() => void this.#copy(remark)}>复制</button></span></div>
          ` : nothing}
        </section>` : nothing}
        <p class="summary-note">${icon('info')}<span>${pending ? remark ? '转账时请填写附言，便于核对到账。' : '请核对订单内容，并在有效时间内完成付款。' : paid ? '付款已确认，请保留本次支付凭证。' : '请返回来源页面重新发起付款。'}</span></p>
      </section>`;
  }

  #renderPending(info: CashierPending) {
    const pay = info.paymentInfo ?? {};
    const channels = this.#availableChannels(info);
    const transfer = ['', 'native', 'bank'].includes((info.payMethod || '').toLowerCase()) &&
      Boolean(pay.accountNo?.trim() || pay.bankName?.trim());
    const disabled = this.#busy || this.walletSubmitted || Boolean(this.connectionMessage) || this.countdown === '订单已过期';
    const paymentQR = Boolean(pay.qrCodeUrl?.trim());
    const paypalAvailable = this.#paypalAvailable(info);
    const paypalAmount = paypalAvailable && info.paypalQuote?.amount !== null && info.paypalQuote?.amount !== undefined
      ? dollars(info.paypalQuote.amount) : '';
    const paypalDescription = this.#paypalDescription(info);
    const paypalLabel = this.connectingChannel === 'paypal' ? '正在连接 PayPal' :
      paypalAvailable ? `PayPal 支付${paypalAmount ? '，' + paypalAmount : ''}` : `PayPal 暂不可用，${paypalDescription}`;
    return html`<div class="payment-heading"><h2 id="payment-title">选择支付方式</h2>
          <div class="payment-subtitle" role="status" aria-live="polite" aria-atomic="true">${this.connectionMessage
            ? html`<span title=${this.connectionMessage}>${this.walletSubmitted ? '付款已提交，连接暂时中断' : '连接暂时中断，将自动重试'}</span><button class="retry-inline" type="button" data-retry aria-busy=${String(this.refreshing)} ?disabled=${this.refreshing} @click=${() => this.#retry()}>${this.refreshing ? '重试中…' : '重试'}</button>`
            : this.walletSubmitted ? '付款已提交，正在确认结果…'
            : this.connectingChannel ? `正在连接${channelName(this.connectingChannel)}…`
            : this.channelError ? html`<span data-channel-error title=${this.channelError}>${this.#channelErrorHint()}</span>`
            : '选择一种方式完成这笔付款。'}</div></div>
        <div class="payment-methods">
          <button class="payment-method" type="button" data-alipay aria-label=${this.connectingChannel === 'alipay' ? '正在连接支付宝' : '支付宝支付'} aria-busy=${String(this.connectingChannel === 'alipay')} ?disabled=${disabled || !channels.includes('alipay')} @click=${() => void this.#gotoGateway('alipay')}>
            <span class="provider-mark alipay" aria-hidden="true"><img src="/payment-icons/alipay.svg" width="26" height="26" alt=""></span><span class="method-copy"><strong>${this.connectingChannel === 'alipay' ? '正在连接…' : '支付宝'}</strong><span>前往支付宝完成付款</span></span>${actionIcon(this.connectingChannel === 'alipay')}
          </button>
          <button class="payment-method" type="button" data-paypal aria-label=${paypalLabel} aria-busy=${String(this.connectingChannel === 'paypal')} title=${!paypalAvailable ? info.paypalQuote?.reason || paypalDescription : nothing} ?disabled=${disabled || !channels.includes('paypal') || !paypalAvailable} @click=${() => void this.#gotoGateway('paypal')}>
            <span class="provider-mark paypal" aria-hidden="true"><img src="/payment-icons/paypal.svg" width="28" height="28" alt=""></span><span class="method-copy"><strong class="method-title"><span>${this.connectingChannel === 'paypal' ? '正在连接…' : 'PayPal'}</span>${paypalAmount ? html`<span class="method-amount" data-paypal-amount>${paypalAmount}</span>` : nothing}</strong><span data-paypal-rate>${paypalDescription}</span></span>${actionIcon(this.connectingChannel === 'paypal')}
          </button>
          <details class="wallet-method" data-wallet><summary class="payment-method" aria-disabled=${String(disabled || !channels.includes('native'))}
            @click=${(event: MouseEvent) => { if (disabled || !channels.includes('native')) event.preventDefault(); }}
            @keydown=${(event: KeyboardEvent) => { if ((disabled || !channels.includes('native')) && (event.key === 'Enter' || event.key === ' ')) event.preventDefault(); }}>
            <span class="provider-mark wallet" aria-hidden="true"><span class="mintweft-mark"></span></span><span class="method-copy"><strong>Mintweft</strong><span>使用 Mintweft 账户完成付款</span></span><span class="wallet-chevron">${icon('chevron')}</span>
          </summary><form class="wallet-content" @submit=${(event: SubmitEvent) => { event.preventDefault(); void this.#payWithWallet(); }}><div class="wallet-fields">
            <div class="field"><div class="pin-label"><label for="walletUser">账号</label><span id="wallet-user-hint">演示账号，可修改</span></div><input id="walletUser" data-wallet-user autocomplete="username" placeholder="请输入账号" .value=${walletUser} required minlength="5" aria-describedby="wallet-user-hint" ?disabled=${this.#busy || this.walletSubmitted}
              @input=${(event: Event) => this.#syncWalletUser(event.currentTarget as HTMLInputElement)} @change=${(event: Event) => this.#syncWalletUser(event.currentTarget as HTMLInputElement)}></div>
            <div class="field"><div class="pin-label"><label for="walletPass">支付密码</label><span id="wallet-pin-hint">6 位数字</span></div>
              <div class="pin-entry"><input class="pin-input" id="walletPass" data-wallet-pass type="password" inputmode="numeric" minlength="6" maxlength="6" pattern="[0-9]{6}" required autocomplete="current-password" spellcheck="false" aria-describedby="wallet-pin-hint" ?disabled=${this.#busy || this.walletSubmitted}
                @beforeinput=${this.#onPinBeforeInput} @input=${this.#onPinInput} @change=${this.#onPinInput} @compositionend=${this.#onPinInput} @paste=${this.#onPinPaste} @click=${this.#onPinClick}
                @focus=${(event: Event) => this.#syncPinPosition(event.currentTarget as HTMLInputElement)} @select=${(event: Event) => this.#syncPinPosition(event.currentTarget as HTMLInputElement)} @keyup=${(event: Event) => this.#syncPinPosition(event.currentTarget as HTMLInputElement)}>
                <div class="pin-boxes" aria-hidden="true">${[0, 1, 2, 3, 4, 5].map(index => html`<span class=${'pin-cell' + (this.#pinSelectionStart === this.#pinSelectionEnd && index === this.#pinIndex ? ' active' : '') + (index >= this.#pinSelectionStart && index < this.#pinSelectionEnd ? ' selected' : '')} data-pin-cell>${index < this.#pinLength ? html`<span class="pin-dot"></span>` : nothing}</span>`)}</div>
              </div>
            </div>
          </div><button class="primary wallet-submit" type="submit" data-wallet-pay aria-busy=${String(this.paying || this.walletSubmitted)} ?disabled=${disabled || !channels.includes('native') || this.#pinLength !== 6 || !this.#walletUserValid}>${this.walletSubmitted ? '正在确认付款…' : this.paying ? '正在支付…' : `确认支付 ${yuan(info.amount)}`}${actionIcon(this.paying || this.walletSubmitted)}</button></form></details>
        </div>
        <div class="qr-section"><div class="qr-surface"><div class="qr-frame" aria-busy=${String(this.#qrState === 'loading')}>
          ${keyed(this.#qrSource + ':' + this.#qrAttempt, html`<img class=${this.#qrState === 'loaded' ? 'qr' : 'qr qr-pending'} data-qr src=${this.#qrSource} alt=${paymentQR ? '付款二维码' : '订单识别二维码'} @load=${(event: Event) => this.#onQRState(event, 'loaded')} @error=${(event: Event) => this.#onQRState(event, 'error')}>`)}
          ${this.#qrState !== 'loaded' ? html`<div class="qr-feedback" role="status">${this.#qrState === 'loading' ? html`<span class="action-spinner" aria-hidden="true"></span><span>加载二维码…</span>` : html`${icon('alert')}<span>二维码加载失败</span><button class="retry-inline" type="button" data-qr-retry @click=${() => this.#retryQR()}>重新加载</button>`}</div>` : nothing}
        </div></div>
          <div class="qr-copy"><h3>${paymentQR ? pay.qrCodeChannel === 'alipay' ? '支付宝扫码支付' : '扫码支付' : '订单识别码'}</h3><p>${paymentQR ? pay.qrCodeChannel === 'alipay' ? '使用支付宝扫描二维码，完成这笔付款。' : '请核对收款信息，使用对应支付应用扫码付款。' : transfer ? '此二维码用于识别当前订单。转账时请填写收款信息中的附言。' : '此二维码仅用于识别当前订单。请通过上方支付渠道完成付款。'}</p></div>
        </div>
      `;
  }

  #renderPaid(info: CashierPaid) {
    return html`<div class="payment-result result-paid">
      <div class="result-heading" role="status"><div class="status-icon success">${icon('check')}</div><h2 id="result-title">支付成功</h2><p class="status-description">付款已确认，您可以返回继续操作。</p></div>
      <div class="receipt-fields" data-details>${kv('支付方式', channelName(info.payMethod))}
        ${info.payMethod === 'paypal' && info.foreignCurrency === 'USD' && info.foreignAmount !== undefined && Number.isFinite(info.foreignAmount) ? kv('实际支付', dollars(info.foreignAmount)) : nothing}
        ${info.payMethod === 'paypal' && info.exchangeRate !== undefined && Number.isFinite(info.exchangeRate) && info.exchangeRate > 0 ? kv('支付汇率', rateLabel(info.exchangeRate)) : nothing}
        ${kv('支付流水号', info.thirdPartyTradeNo)}${kv('支付时间', dateTime(info.payAt))}</div>
      <button class="primary status-action" type="button" data-done @click=${() => this.#leave()}>完成${icon('arrow')}</button>
    </div>`;
  }

  #renderClosed(info: CashierClosed) {
    const expired = info.status === 'expired';
    return html`<div class="payment-result result-closed">
      <div class="status-icon warning" data-icon>${icon(expired ? 'clock' : 'alert')}</div><h2 id="result-title" data-title>${expired ? '订单已超时关闭' : '本次支付未完成'}</h2>
      <p class="status-description" data-message>${expired ? '订单已超过有效支付时间，请返回来源页面重新下单。' : '支付通道未能完成本次交易，请返回来源页面后重试。'}</p>
      <button class="primary status-action" type="button" data-back @click=${() => this.#leave()}>返回来源页面${icon('arrow')}</button>
    </div>`;
  }

  #renderError() {
    return html`<div class="payment-result result-error" role="alert">
      <div class="status-icon error">${icon('alert')}</div><h2 id="result-title">无法打开收银台</h2><p class="status-description" data-message>${this.errorMessage}</p>
      <button class="primary status-action" type="button" data-retry @click=${() => this.#retry()}>重新加载${icon('arrow')}</button>
    </div>`;
  }
}

customElements.define('sp-cashier', SpCashier);
