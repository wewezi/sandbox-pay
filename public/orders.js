/**
 * 订单列表页。行、统计卡、退款弹窗的结构都在 orders.html 的 <template> 里，
 * 这边只 clone + 填字段。
 */

import {
  api,
  must,
  mustBe,
  el,
  fromTemplate,
  openModal,
  yuan,
  percent,
  count,
  timeEl,
  qs,
  fail,
  ok,
  isAbort,
  latest,
  requireAuth,
  renderTopbar,
  renderPager,
  confirmDialog,
} from '/app.js';

/**
 * @typedef {import('./api-types.d.ts').AdminStats} AdminStats
 * @typedef {import('./api-types.d.ts').AdminTransaction} AdminTransaction
 * @typedef {import('./api-types.d.ts').AdminTxPage} AdminTxPage
 * @typedef {import('./api-types.d.ts').AdminTxStatus} AdminTxStatus
 */

const PAGE_SIZE = 20;

/** @type {Record<string, string>} */
const CHANNEL_LABEL = {
  ALIPAY: '支付宝',
  PAYPAL: 'PayPal',
  NATIVE: '自有',
  WECHAT: '微信',
  USDT: 'USDT',
};

/** @type {Record<AdminTxStatus, readonly [label: string, kind: string]>} */
const STATUS_LABEL = {
  PENDING: ['待支付', 'warn'],
  SUCCESS: ['已支付', 'ok'],
  FAILED: ['失败', 'err'],
  REFUNDING: ['退款中', 'info'],
  REFUNDED: ['已退款', ''],
};

/** 存管核销的默认凭证，后端要求账号 ≥5 位、密码 ≥6 位。 */
const WALLET_USER = 'demo_buyer@sandbox-pay.local';
const WALLET_PASS = '123456';

/** 浮点比较的容差。金额是元，比到分就够。 */
const EPS = 0.001;

let state = { page: 1, pageSize: PAGE_SIZE, total: 0 };

/**
 * 列表和统计各自一条「后发先至」防护。
 *
 * 分开是因为它们是两个独立的请求流：翻页只重发列表，改完数据
 * （退款 / 删除）两个都重发。共用一个 signal 会让翻页把统计也掐掉。
 */
const listSignal = latest();
const statsSignal = latest();

function filters() {
  return {
    keyword: mustBe('#keyword', HTMLInputElement).value.trim(),
    status: mustBe('#status', HTMLSelectElement).value,
    payMethod: mustBe('#payMethod', HTMLSelectElement).value,
    dateFrom: mustBe('#dateFrom', HTMLInputElement).value,
    dateTo: mustBe('#dateTo', HTMLInputElement).value,
  };
}

function bindFilters() {
  const form = mustBe('#filters', HTMLFormElement);
  form.addEventListener('submit', (e) => {
    e.preventDefault();
    void load(1);
  });
  form.addEventListener('reset', () => {
    // reset 先于表单清空触发，等一轮再读取
    setTimeout(() => void load(1), 0);
  });
}

async function loadStats() {
  try {
    const s = /** @type {AdminStats} */ (
      await api.get('/admin/stats', { signal: statsSignal() })
    );
    /** @type {readonly [string, string][]} */
    const cards = [
      ['成交金额', yuan(s.amount)],
      ['成功笔数', `${count(s.successCount)} 笔`],
      // null = 一笔都没有，给中性占位而不是「0.0%」
      ['成功率', s.successRate === null ? '—' : percent(s.successRate)],
    ];
    must('#stats').replaceChildren(...cards.map(statCard));
  } catch (err) {
    if (isAbort(err)) return;
    fail(err);
  }
}

/**
 * @param {readonly [string, string]} entry
 * @returns {DocumentFragment}
 */
function statCard([k, v]) {
  const frag = fromTemplate('tpl-stat');
  must('[data-k]', frag).textContent = k;
  must('[data-v]', frag).textContent = v;
  return frag;
}

/** @param {number} page */
async function load(page) {
  state.page = page;
  try {
    const res = /** @type {AdminTxPage} */ (
      await api.get(
        '/admin/transactions' + qs({ ...filters(), page, pageSize: PAGE_SIZE }),
        { signal: listSignal() },
      )
    );
    state = { page: res.page, pageSize: res.pageSize, total: res.total };
    must('#empty').hidden = res.items.length > 0;
    must('#rows').replaceChildren(...res.items.map(row));
    renderPager(must('#pager'), state, (p) => void load(p));
  } catch (err) {
    // 被后一次查询掐掉了，那次会自己渲染，这里什么都不做
    if (isAbort(err)) return;
    fail(err);
  }
}

/**
 * 还能退多少，单位元。
 * @param {AdminTransaction} t
 */
const refundable = (t) => Number((t.amount - t.refundedAmount).toFixed(2));

/**
 * @param {AdminTransaction} t
 * @returns {DocumentFragment}
 */
function row(t) {
  const frag = fromTemplate('tpl-order-row');

  must('[data-order]', frag).textContent = t.orderNo;
  must('[data-product]', frag).textContent = t.productName || '—';
  must('[data-channel]', frag).textContent =
    CHANNEL_LABEL[t.channel] || t.channel;
  must('[data-amount]', frag).textContent = yuan(t.amount);
  must('[data-refunded]', frag).textContent =
    t.refundedAmount > 0 ? yuan(t.refundedAmount) : '—';
  must('[data-fee]', frag).textContent = yuan(t.fee);

  const [label, kind] = STATUS_LABEL[t.status] ?? [t.status, ''];
  const status = must('[data-status]', frag);
  status.textContent = label;
  if (kind) status.classList.add(kind);

  // <sp-time> 自己会走：列表开着不动，「1分钟前」到点自己变
  must('[data-time]', frag).replaceChildren(timeEl(t.createdAt));

  const acts = must('[data-acts]', frag);
  // 存管核销只对 native 待支付订单开放，支付宝 / PayPal 后端会拒
  if (t.status === 'PENDING' && t.payMethod === 'native') {
    acts.appendChild(
      el('button', {
        class: 'link',
        text: '确认收款',
        onclick: () => void confirmPay(t),
      }),
    );
  }
  if (t.status === 'SUCCESS' && refundable(t) > EPS) {
    acts.appendChild(
      el('button', { class: 'link', text: '退款', onclick: () => refund(t) }),
    );
  }
  acts.appendChild(
    el('button', {
      class: 'link danger',
      text: '删除',
      onclick: () => void remove(t),
    }),
  );

  return frag;
}

/** 改完数据后刷新列表和统计。 */
function reload() {
  void load(state.page);
  void loadStats();
}

/** @param {AdminTransaction} t */
async function confirmPay(t) {
  const yes = await confirmDialog({
    title: '确认收款',
    message: `将 ${t.orderNo}（${yuan(t.amount)}）按存管到账核销，金额以订单原值入账。`,
    confirmText: '确认到账',
  });
  if (!yes) return;
  try {
    await api.post('/native-pay/confirm', {
      orderNo: t.orderNo,
      walletUser: WALLET_USER,
      walletPass: WALLET_PASS,
    });
    ok('已确认收款');
    reload();
  } catch (err) {
    fail(err);
  }
}

/** @param {AdminTransaction} t */
async function remove(t) {
  const yes = await confirmDialog({
    title: '删除订单',
    message: `删除 ${t.orderNo} 后不可恢复。`,
    confirmText: '删除',
    danger: true,
  });
  if (!yes) return;
  try {
    await api.del('/admin/transactions/' + encodeURIComponent(t.orderNo));
    ok('已删除');
    reload();
  } catch (err) {
    fail(err);
  }
}

/**
 * 退款弹窗。金额一律按人民币元填，PayPal 由后端按订单汇率折算成 USD。
 *
 * @param {AdminTransaction} t
 */
function refund(t) {
  const max = refundable(t);
  const frag = fromTemplate('tpl-refund');
  const dialog = mustBe('dialog', HTMLDialogElement, frag);

  must('[data-summary]', dialog).textContent =
    `${t.orderNo} · 可退 ${yuan(max)}`;

  const form = mustBe('[data-form]', HTMLFormElement, dialog);
  const amount = mustBe('[data-amount]', HTMLInputElement, dialog);
  const reason = mustBe('[data-reason]', HTMLInputElement, dialog);
  const submit = mustBe('[data-submit]', HTMLButtonElement, dialog);

  // 上限写进 max 属性，超额由浏览器拦 —— 不满足约束时下面的
  // submit 事件根本不会触发，气泡自己指到输入框上。
  amount.max = String(max);
  amount.value = String(max);

  const dismiss = openModal(dialog);
  mustBe('[data-cancel]', HTMLButtonElement, dialog).addEventListener(
    'click',
    dismiss,
  );
  form.addEventListener('submit', (e) => {
    e.preventDefault();
    void send();
  });
  amount.focus();

  async function send() {
    const value = Number(amount.value);
    submit.disabled = true;
    submit.textContent = '提交中…';
    try {
      await api.post('/admin/refund', {
        orderNo: t.orderNo,
        amount: value,
        ...(reason.value.trim() ? { reason: reason.value.trim() } : {}),
      });
      dismiss();
      ok('退款已提交');
      reload();
    } catch (err) {
      fail(err);
      submit.disabled = false;
      submit.textContent = '提交退款';
    }
  }
}

// 启动块放最后：load() 会读 state，而 let 声明有 TDZ，
// 放文件顶部会直接抛 Cannot access 'state' before initialization。
if (requireAuth()) {
  renderTopbar('/orders.html');
  bindFilters();
  void loadStats();
  void load(1);
}
