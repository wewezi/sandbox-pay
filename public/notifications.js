/**
 * 通知队列页。行结构在 notifications.html 的 <template> 里，这边只填字段。
 */

import {
  api,
  must,
  mustBe,
  el,
  fromTemplate,
  ok,
  fail,
  isAbort,
  latest,
  requireAuth,
  renderTopbar,
  renderPager,
  qs,
  timeEl,
} from '/app.js';

/**
 * @typedef {import('./api-types.d.ts').AdminNotification} AdminNotification
 * @typedef {import('./api-types.d.ts').AdminNotifyPage} AdminNotifyPage
 */

const PAGE_SIZE = 50;
const state = { page: 1, pageSize: PAGE_SIZE, total: 0, status: '' };

/** 「后发先至」防护：连点查询 / 刷新时，先发的响应不会盖掉后发的。 */
const listSignal = latest();

/**
 * NotifyStatus 数字 → [文案, 徽章 class]。
 * 0=Pending, 1=Success, 2=Failed, 3=Exhausted，见 internal/db/notify.go。
 *
 * @type {Record<number, readonly [label: string, kind: string]>}
 */
const STATUS = {
  0: ['待发送', 'info'],
  1: ['成功', 'ok'],
  2: ['失败重试中', 'warn'],
  3: ['重试耗尽', 'err'],
};

/** 成功状态不给重发按钮 —— 重发一次会让接入方收到重复通知。 */
const SUCCESS = 1;

function init() {
  const filters = mustBe('#filters', HTMLFormElement);
  const status = mustBe('#status', HTMLSelectElement);

  filters.addEventListener('submit', (e) => {
    e.preventDefault();
    state.status = status.value;
    state.page = 1;
    void load();
  });

  mustBe('#refresh', HTMLButtonElement).addEventListener(
    'click',
    () => void load(),
  );
  void load();
}

async function load() {
  const rows = must('#rows');
  try {
    const data = /** @type {AdminNotifyPage} */ (
      await api.get(
        '/admin/notifications' +
          qs({
            page: state.page,
            pageSize: state.pageSize,
            status: state.status,
          }),
        { signal: listSignal() },
      )
    );
    state.total = data.total;
    state.page = data.page;
    state.pageSize = data.pageSize;

    must('#empty').hidden = data.items.length > 0;
    // 一次换掉整个 tbody，省掉「先清空再逐个 append」的中间态
    rows.replaceChildren(...data.items.map(row));
    renderPager(must('#pager'), state, (page) => {
      state.page = page;
      void load();
    });
  } catch (err) {
    // 被后一次查询掐掉了，那次会自己渲染
    if (isAbort(err)) return;
    fail(err);
  }
}

/**
 * @param {AdminNotification} n
 * @returns {DocumentFragment}
 */
function row(n) {
  const frag = fromTemplate('tpl-notify-row');

  must('[data-order]', frag).textContent = n.orderNo || '—';
  must('[data-url]', frag).textContent = n.url || '—';
  must('[data-retry]', frag).textContent = String(n.retryCount);
  must('[data-error]', frag).textContent = n.lastError || '—';
  must('[data-attempt]', frag).replaceChildren(timeEl(n.lastAttemptAt));
  must('[data-created]', frag).replaceChildren(timeEl(n.createdAt));

  const [label, kind] = STATUS[n.status] ?? [String(n.status), ''];
  const status = must('[data-status]', frag);
  status.textContent = label;
  if (kind) status.classList.add(kind);

  if (n.status !== SUCCESS) {
    must('[data-acts]', frag).appendChild(
      el('button', {
        class: 'link',
        text: '重发',
        onclick: () => void replay(n),
      }),
    );
  }

  return frag;
}

/** @param {AdminNotification} n */
async function replay(n) {
  try {
    await api.post(`/admin/notifications/${n.id}/replay`);
    ok('已重新入队，等待发送');
    await load();
  } catch (err) {
    fail(err);
  }
}

// 启动块放最后：init() → load() 会读 state，const 声明有 TDZ，
// 放文件顶部会直接抛 Cannot access 'state' before initialization。
if (requireAuth()) {
  renderTopbar('/notifications.html');
  init();
}
