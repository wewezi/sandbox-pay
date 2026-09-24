/**
 * 收银台。公开页面，不需要登录。
 *
 * 整页是一个 <sp-cashier>：自己拉数据、自己轮询、自己渲染。
 * 状态机 loading → pending / paid / expired / failed / error，
 * 每个状态一份 <template>（见 cashier.html）。
 *
 * pending 时 5s 轮询订单状态、1s 刷新倒计时。两个定时器和在飞的请求
 * 都挂在 AbortController 上，元素断开时一起收掉 —— 原来是手工
 * clearInterval 两个模块级变量，轮询的 fetch 还会在页面卸载后落地。
 */

import {
  api,
  must,
  mustBe,
  fromTemplate,
  ok,
  fail,
  toast,
  yuan,
  dateTime,
  mmss,
  mountFloatingThemeToggle,
} from '/app.js';

/**
 * @typedef {import('./api-types.d.ts').CashierInfo} CashierInfo
 * @typedef {import('./api-types.d.ts').CashierPending} CashierPending
 * @typedef {import('./api-types.d.ts').CashierPaid} CashierPaid
 * @typedef {import('./api-types.d.ts').CashierClosed} CashierClosed
 * @typedef {import('./api-types.d.ts').GatewayPayResult} GatewayPayResult
 */

const POLL_MS = 5000;
const TICK_MS = 1000;

const WALLET_USER = 'demo_buyer@sandbox-pay.local';
const WALLET_PASS = '123456';

/** @param {string} m */
function channelName(m) {
  if (m === 'alipay') return '支付宝';
  if (m === 'paypal') return 'PayPal';
  return '沙箱钱包';
}

/** 关掉当前页：能关就关，否则退回上一页，都不行就回首页。 */
function leave() {
  if (window.opener) return window.close();
  if (history.length > 1) return history.back();
  location.href = '/';
}

/**
 * 一行「键：值」。值为空时整行不渲染。
 *
 * @param {string} k
 * @param {string | number | null | undefined} v
 * @returns {DocumentFragment | null}
 */
function kv(k, v) {
  if (!v) return null;
  const frag = fromTemplate('tpl-kv');
  must('[data-k]', frag).textContent = k;
  must('[data-v]', frag).textContent = String(v);
  return frag;
}

/**
 * 把 kv() 的结果填进容器，跳过空行。
 *
 * @param {HTMLElement} host
 * @param {(DocumentFragment | null)[]} rows
 */
function fillDetails(host, rows) {
  host.replaceChildren(...rows.filter((r) => r !== null));
}

class SpCashier extends HTMLElement {
  /** @type {AbortController | null} */
  #ac = null;
  /** @type {CashierInfo | null} */
  #info = null;
  /** @type {ReturnType<typeof setInterval> | null} */
  #poll = null;
  /** @type {ReturnType<typeof setInterval> | null} */
  #tick = null;
  /** @type {HTMLElement | null} */
  #countdown = null;
  /**
   * 只在状态变化时重渲染，否则 5s 一次的轮询会清掉
   * 用户正在输入的钱包账号。
   * @type {string | null}
   */
  #rendered = null;
  #switching = false;
  #paying = false;

  get #orderNo() {
    return new URLSearchParams(location.search).get('orderNo') || '';
  }

  connectedCallback() {
    this.#ac = new AbortController();

    if (!this.#orderNo) {
      this.#renderError('支付链接缺少订单号，请重新获取支付链接');
      return;
    }

    this.#consumeReturnStatus();
    void this.#fetchInfo();
    this.#poll = setInterval(() => void this.#fetchInfo(), POLL_MS);
  }

  disconnectedCallback() {
    // 请求和定时器一起收。abort 之后在飞的 fetch 会以 AbortError 结束，
    // #fetchInfo 里按取消处理，不会再往已经拆掉的 DOM 上写东西。
    this.#ac?.abort();
    this.#stopPoll();
    this.#stopTick();
  }

  #stopPoll() {
    if (this.#poll) clearInterval(this.#poll);
    this.#poll = null;
  }

  #stopTick() {
    if (this.#tick) clearInterval(this.#tick);
    this.#tick = null;
  }

  /** PayPal 回调带 ?status=... 回来，提示一次就把参数抹掉，刷新时不再弹。 */
  #consumeReturnStatus() {
    const params = new URLSearchParams(location.search);
    const status = params.get('status');
    if (!status) return;
    if (status === 'cancelled') toast('已取消 PayPal 支付');
    else if (status === 'failed') fail('PayPal 支付未完成');
    params.delete('status');
    const rest = params.toString();
    history.replaceState(
      null,
      '',
      location.pathname + (rest ? '?' + rest : ''),
    );
  }

  async #fetchInfo() {
    try {
      const data = /** @type {CashierInfo} */ (
        await api.get(
          `/native-pay/cashier?orderNo=${encodeURIComponent(this.#orderNo)}`,
          { signal: this.#ac?.signal },
        )
      );
      this.#info = data;
      if (data.status !== 'pending') this.#stopPoll();
      if (data.status !== this.#rendered) this.#render();
    } catch (err) {
      // 元素已经断开、请求被 abort：什么都不做，DOM 也许已经没了
      if (err instanceof DOMException && err.name === 'AbortError') return;
      this.#stopPoll();
      this.#renderError(err instanceof Error ? err.message : '订单查询失败');
    }
  }

  /** @param {(Node | null)[]} nodes */
  #mount(nodes) {
    this.replaceChildren(...nodes.filter((n) => n !== null));
  }

  #render() {
    const info = this.#info;
    if (!info) return;
    this.#rendered = info.status;
    this.#stopTick();
    this.#countdown = null;

    if (info.status === 'paid') return this.#renderPaid(info);
    if (info.status === 'pending') return this.#renderPending(info);
    this.#renderClosed(info);
  }

  /** @param {CashierPending} info */
  #renderPending(info) {
    const frag = fromTemplate('tpl-pending');
    const pay = info.paymentInfo ?? {};
    const remark = pay.remark || `WP${info.orderNo}`;

    must('[data-product]', frag).textContent = info.productName;
    must('[data-amount]', frag).textContent = yuan(info.amount);
    this.#countdown = must('[data-countdown]', frag);

    // 后端没给二维码时退到公共生成服务，扫出来是订单号
    const qr = mustBe('[data-qr]', HTMLImageElement, frag);
    qr.src =
      (pay.qrCodeUrl || '').trim() ||
      'https://api.qrserver.com/v1/create-qr-code/?size=300x300&margin=8&data=' +
        encodeURIComponent(`SANDBOX_PAY_ORDER_${info.orderNo}`);

    const alipay = mustBe('[data-alipay]', HTMLButtonElement, frag);
    alipay.textContent = `支付宝支付 ${yuan(info.amount)}`;
    alipay.addEventListener(
      'click',
      () => void this.#gotoGateway('alipay', alipay),
    );

    const paypal = mustBe('[data-paypal]', HTMLButtonElement, frag);
    paypal.addEventListener(
      'click',
      () => void this.#gotoGateway('paypal', paypal),
    );

    // native 订单才允许钱包核销，支付宝 / PayPal 订单后端会直接拒绝
    const method = (info.payMethod || '').toLowerCase();
    const wallet = must('[data-wallet]', frag);
    if (!method || method === 'native' || method === 'bank') {
      const user = mustBe('[data-wallet-user]', HTMLInputElement, wallet);
      const pass = mustBe('[data-wallet-pass]', HTMLInputElement, wallet);
      const btn = mustBe('[data-wallet-pay]', HTMLButtonElement, wallet);
      btn.textContent = `确认支付 ${yuan(info.amount)}`;
      btn.addEventListener(
        'click',
        () => void this.#payWithWallet(btn, user, pass),
      );
    } else {
      wallet.remove();
    }

    fillDetails(must('[data-details]', frag), [
      kv('订单号', info.orderNo),
      kv('收款户名', pay.accountName),
      kv('收款账号', pay.accountNo),
      kv('开户银行', pay.bankName),
    ]);

    must('[data-remark]', frag).textContent = remark;
    must('[data-copy]', frag).addEventListener(
      'click',
      () => void this.#copy(remark),
    );

    this.#mount([frag, fromTemplate('tpl-note')]);
    this.#startTick(info);
  }

  /** @param {CashierPaid} info */
  #renderPaid(info) {
    const frag = fromTemplate('tpl-paid');
    must('[data-amount]', frag).textContent = yuan(info.amount);
    must('[data-product]', frag).textContent = info.productName;

    fillDetails(must('[data-details]', frag), [
      kv('订单号', info.orderNo),
      kv('支付方式', channelName(info.payMethod)),
      kv('支付流水号', info.thirdPartyTradeNo),
      kv('支付时间', dateTime(info.payAt)),
    ]);

    must('[data-done]', frag).addEventListener('click', leave);
    this.#mount([frag, fromTemplate('tpl-note')]);
  }

  /** @param {CashierClosed} info */
  #renderClosed(info) {
    const expired = info.status === 'expired';
    const frag = fromTemplate('tpl-closed');

    must('[data-icon]', frag).textContent = expired ? '⏰' : '⚠️';
    must('[data-title]', frag).textContent = expired
      ? '订单已超时关闭'
      : '本次支付未完成';
    must('[data-message]', frag).textContent = expired
      ? '订单超过有效支付时间后会自动关闭，请返回来源页面重新下单。'
      : '支付通道未能完成本次交易，请返回来源页面后重试。';

    fillDetails(must('[data-details]', frag), [kv('订单号', info.orderNo)]);
    must('[data-back]', frag).addEventListener('click', leave);
    this.#mount([frag]);
  }

  /** @param {string} message */
  #renderError(message) {
    this.#rendered = 'error';
    this.#stopTick();
    this.#countdown = null;

    const frag = fromTemplate('tpl-error');
    must('[data-message]', frag).textContent = message;
    must('[data-retry]', frag).addEventListener('click', () =>
      location.reload(),
    );
    this.#mount([frag]);
  }

  /**
   * 倒计时。到 0 就拉一次接口，让后端把订单判成 expired。
   * @param {CashierPending} info
   */
  #startTick(info) {
    this.#stopTick();
    if (!info.expireAt) return;
    const deadline = new Date(info.expireAt).getTime();

    const update = () => {
      const node = this.#countdown;
      if (!node) return;
      const text = mmss(Math.floor((deadline - Date.now()) / 1000));
      if (text === null) {
        node.textContent = '订单已过期';
        this.#stopTick();
        void this.#fetchInfo();
        return;
      }
      node.textContent = `支付剩余时间 ${text}`;
    };

    update();
    this.#tick = setInterval(update, TICK_MS);
  }

  /** @param {string} value */
  async #copy(value) {
    try {
      await navigator.clipboard.writeText(value);
      ok('已复制');
    } catch {
      fail('复制失败，请手动选中复制');
    }
  }

  /**
   * 调真实网关。支付宝返回自提交表单，PayPal 返回跳转 URL。
   *
   * @param {'alipay' | 'paypal'} channel
   * @param {HTMLButtonElement} btn
   */
  async #gotoGateway(channel, btn) {
    if (this.#switching) return;
    this.#switching = true;
    const label = btn.textContent;
    btn.disabled = true;
    btn.textContent = '正在连接…';
    try {
      const payload = /** @type {GatewayPayResult} */ (
        await api.post(
          '/native-pay/switch-channel',
          { orderNo: this.#orderNo, channel },
          { signal: this.#ac?.signal },
        )
      );
      const data = payload?.data;
      if (!data) throw new Error('网关未返回跳转信息');

      if (channel === 'paypal' || data.startsWith('http')) {
        location.href = data;
        return;
      }

      // 支付宝给的是一段带 <script> 自动提交的 HTML，CSP 不允许执行内联脚本，
      // 所以剥掉 script、只把 form 拿过来手动 submit。
      const doc = new DOMParser().parseFromString(data, 'text/html');
      doc.querySelectorAll('script').forEach((s) => s.remove());
      const form = doc.querySelector('form');
      if (!form) throw new Error('支付宝返回的表单无效');
      document.body.appendChild(form);
      form.submit();
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') return;
      fail(err instanceof Error ? err.message : '网关暂时不可用');
      btn.disabled = false;
      btn.textContent = label;
    } finally {
      this.#switching = false;
    }
  }

  /**
   * @param {HTMLButtonElement} btn
   * @param {HTMLInputElement} user
   * @param {HTMLInputElement} pass
   */
  async #payWithWallet(btn, user, pass) {
    if (this.#paying) return;
    this.#paying = true;
    const label = btn.textContent;
    btn.disabled = true;
    btn.textContent = '正在支付…';
    try {
      await api.post(
        '/native-pay/sandbox-confirm',
        {
          orderNo: this.#orderNo,
          walletUser: user.value.trim() || WALLET_USER,
          walletPass: pass.value || WALLET_PASS,
        },
        { signal: this.#ac?.signal },
      );
      ok('支付成功');
      await this.#fetchInfo();
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') return;
      fail(err instanceof Error ? err.message : '支付失败');
      btn.disabled = false;
      btn.textContent = label;
    } finally {
      this.#paying = false;
    }
  }
}

customElements.define('sp-cashier', SpCashier);

mountFloatingThemeToggle();
