/**
 * 分组与 API Key 管理页。
 *
 * 两个 custom element，都不带 shadow DOM（原因见 groups.html 的注释）：
 *   <sp-secret>        一把凭证：打码 / 显示 / 复制。自己管展开状态。
 *   <sp-group-card> 一个分组：名字、启停、Key 列表、签发表单。
 *
 * 结构全在 groups.html 的 <template> 里，这边只 clone + 填字段。
 */

import {
  api,
  fromTemplate,
  must,
  mustBe,
  dateTime,
  ok,
  fail,
  requireAuth,
  renderTopbar,
  confirmDialog,
} from '/app.js';

/**
 * 接口返回的形状写在 api-types.d.ts —— 它是一份独立声明：后端是 Go，
 * 前端的 tsconfig 只看 public/，引不进 Go 的 struct。为什么日期是
 * string 而不是 Date，也在那边的头注释里。
 *
 * @typedef {import('./api-types.d.ts').GroupWithKeys} GroupWithKeys
 * @typedef {import('./api-types.d.ts').GroupKey} GroupKey
 */

// ── <sp-secret> ────────────────────────────────

/**
 * 一把凭证的显示控件。
 *
 * 展开状态是组件自己的字段，不再是模块级的 `revealed` Set ——
 * 原来那个 Set 存在的唯一原因是「整页重渲染后要记住哪几把展开着」，
 * 现在状态跟着元素本身活，重渲染不销毁元素就不会丢。
 */
class SpSecret extends HTMLElement {
  /** @type {string} */
  #value = '';
  #shown = false;
  /** @type {HTMLElement | null} */
  #valueEl = null;
  /** @type {HTMLElement | null} */
  #revealEl = null;

  connectedCallback() {
    if (this.#valueEl) return; // 已经建好了，移动位置不重建

    this.append(fromTemplate('tpl-secret'));
    this.#valueEl = must('[data-value]', this);
    this.#revealEl = must('[data-reveal]', this);

    this.#revealEl.addEventListener('click', () => {
      this.#shown = !this.#shown;
      this.#sync();
    });
    must('[data-copy]', this).addEventListener(
      'click',
      () => void this.#copy(),
    );
    this.#sync();
  }

  /** @param {string} v */
  set value(v) {
    this.#value = v;
    this.#sync();
  }

  get value() {
    return this.#value;
  }

  #sync() {
    if (!this.#valueEl || !this.#revealEl) return;
    this.#valueEl.textContent = this.#shown ? this.#value : mask(this.#value);
    this.#revealEl.textContent = this.#shown ? '隐藏' : '显示';
  }

  /** 新签发的 Key 调一次，明文直接展开给人抄 */
  reveal() {
    this.#shown = true;
    this.#sync();
  }

  async #copy() {
    try {
      await navigator.clipboard.writeText(this.#value);
      ok('已复制');
    } catch {
      // 非 https 或权限被拒时 clipboard 不可用，退回 prompt 让人自己抄
      window.prompt('剪贴板不可用，请手动复制', this.#value);
    }
  }
}

/**
 * 前缀 + 4 位 hex + 省略 + 末 4 位，一眼还能对上是哪把。
 * @param {string} key
 */
function mask(key) {
  if (key.length <= 16) return '••••••••';
  return key.slice(0, 16) + '…' + key.slice(-4);
}

// ── <sp-group-card> ─────────────────────────

/**
 * 一个分组卡片。
 *
 * 数据靠 property 传（`card.group = m`）而不是 attribute ——
 * attribute 只能是字符串，塞对象得 JSON 来回序列化。
 *
 * 操作不自己发请求，而是冒泡 CustomEvent 交给页面处理：
 * 组件不需要知道接口路径长什么样，页面也只在一个地方 reload。
 */
class SpGroupCard extends HTMLElement {
  /** @type {GroupWithKeys | null} */
  #group = null;

  /** @param {GroupWithKeys} m */
  set group(m) {
    this.#group = m;
    this.#render();
  }

  #render() {
    const m = this.#group;
    if (!m) return;

    const frag = fromTemplate('tpl-group-card');

    must('[data-name]', frag).textContent = m.name;

    const status = must('[data-status]', frag);
    status.textContent = m.isActive ? '启用' : '已停用';
    status.classList.add(m.isActive ? 'ok' : 'err');

    const toggle = must('[data-toggle-group]', frag);
    toggle.textContent = m.isActive ? '停用分组' : '启用分组';
    if (m.isActive) toggle.classList.add('danger');
    toggle.addEventListener('click', () => {
      this.#emit('toggle-group', { group: m });
    });

    must('[data-keys]', frag).append(...m.keys.map((k) => this.#keyRow(k)));

    // label 和 input 的 for/id 要对上，同页面多个卡片得各自唯一
    const label = must('[data-issue-label]', frag);
    const input = mustBe('[data-issue-input]', HTMLInputElement, frag);
    label.setAttribute('for', `keyName-${m.id}`);
    input.id = `keyName-${m.id}`;

    const form = must('[data-issue-form]', frag);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      this.#emit('issue-key', { group: m, name: input.value.trim() });
    });

    this.replaceChildren(frag);
  }

  /**
   * @param {GroupKey} k
   * @returns {DocumentFragment}
   */
  #keyRow(k) {
    const frag = fromTemplate('tpl-key-row');

    must('[data-key-name]', frag).textContent = k.name;

    const secret = /** @type {SpSecret} */ (must('sp-secret', frag));
    secret.value = k.key;
    secret.dataset.keyId = String(k.id);

    const status = must('[data-key-status]', frag);
    status.textContent = k.isActive ? '启用' : '已停用';
    status.classList.add(k.isActive ? 'ok' : 'err');

    must('[data-key-used]', frag).textContent = k.lastUsedAt
      ? dateTime(k.lastUsedAt)
      : '从未';

    const toggle = must('[data-toggle-key]', frag);
    toggle.textContent = k.isActive ? '停用' : '启用';
    if (k.isActive) toggle.classList.add('danger');
    toggle.addEventListener('click', () =>
      this.#emit('toggle-key', { key: k }),
    );

    return frag;
  }

  /**
   * @param {string} type
   * @param {Record<string, unknown>} detail
   */
  #emit(type, detail) {
    this.dispatchEvent(
      new CustomEvent(type, { detail, bubbles: true, composed: true }),
    );
  }
}

customElements.define('sp-secret', SpSecret);
customElements.define('sp-group-card', SpGroupCard);

// ── 页面 ───────────────────────────────────────

if (requireAuth()) {
  renderTopbar('/groups.html');
  bindEvents();
  bindCreateGroup();
  void load();
}

/** 新签发 Key 的 id，渲染后把对应的 <sp-secret> 展开 */
let revealNext = -1;

async function load() {
  try {
    const groups = /** @type {GroupWithKeys[]} */ (
      await api.get('/admin/groups')
    );
    const host = must('#groups');
    host.className = 'stack';
    host.replaceChildren(
      ...groups.map((m) => {
        const card = /** @type {SpGroupCard} */ (
          document.createElement('sp-group-card')
        );
        card.group = m;
        return card;
      }),
    );

    if (revealNext >= 0) {
      const secret = /** @type {SpSecret | null} */ (
        host.querySelector(`sp-secret[data-key-id="${revealNext}"]`)
      );
      secret?.reveal();
      revealNext = -1;
    }
  } catch (err) {
    fail(err);
  }
}

/**
 * 卡片的操作事件在这里统一处理。
 * 事件是冒泡的，所以监听容器一个就够，不用给每张卡片挂。
 */
function bindEvents() {
  const host = must('#groups');

  host.addEventListener('toggle-group', (e) => {
    const { group } = detailOf(e);
    if (group) void toggleGroup(group);
  });

  host.addEventListener('toggle-key', (e) => {
    const { key } = detailOf(e);
    if (key) void toggleKey(key);
  });

  host.addEventListener('issue-key', (e) => {
    const { group, name } = detailOf(e);
    if (group) void issueKey(group.id, name ?? '');
  });
}

/**
 * CustomEvent 的 detail 是 any，在这里一次性收窄，
 * 别在三个 handler 里各断言一遍 —— 那样 any 会渗到每个调用点。
 *
 * @param {Event} e
 * @returns {{ group?: GroupWithKeys, key?: GroupKey, name?: string }}
 */
function detailOf(e) {
  if (!(e instanceof CustomEvent)) return {};
  const detail = /** @type {unknown} */ (e.detail);
  if (typeof detail !== 'object' || detail === null) return {};
  return detail;
}

/** @param {GroupWithKeys} m */
async function toggleGroup(m) {
  if (m.isActive) {
    const yes = await confirmDialog({
      title: '停用分组',
      message: `停用后「${m.name}」名下所有 Key 都调不通网关。已有订单的回调签名不受影响。`,
      confirmText: '确认停用',
      danger: true,
    });
    if (!yes) return;
  }
  try {
    await api.post(`/admin/groups/${m.id}/toggle`);
    ok(m.isActive ? '已停用' : '已启用');
    await load();
  } catch (err) {
    fail(err);
  }
}

/** @param {GroupKey} k */
async function toggleKey(k) {
  if (k.isActive) {
    const yes = await confirmDialog({
      title: '停用 API Key',
      message: '停用后这把 Key 立刻调不通。已用它下的单，回调仍用它签名。',
      confirmText: '确认停用',
      danger: true,
    });
    if (!yes) return;
  }
  try {
    await api.post(`/admin/api-keys/${k.id}/toggle`);
    ok(k.isActive ? '已停用' : '已启用');
    await load();
  } catch (err) {
    fail(err);
  }
}

/**
 * @param {number} groupId
 * @param {string} name
 */
async function issueKey(groupId, name) {
  try {
    const key = /** @type {GroupKey} */ (
      await api.post(`/admin/groups/${groupId}/keys`, {
        name: name || undefined,
      })
    );
    revealNext = key.id;
    ok('已签发，明文只这一次完整展示');
    await load();
  } catch (err) {
    fail(err);
  }
}

function bindCreateGroup() {
  const form = mustBe('#createGroup', HTMLFormElement);
  const nameInput = mustBe('#groupName', HTMLInputElement);
  const keyInput = mustBe('#firstKeyName', HTMLInputElement);

  // handler 本身保持同步：addEventListener 期望返回 void，
  // 直接给 async 函数等于把 Promise 丢在这里没人管（拒绝会变成
  // unhandledrejection 而不是走下面的 catch）。
  form.addEventListener('submit', (e) => {
    e.preventDefault();
    void create();
  });

  async function create() {
    const name = nameInput.value.trim();
    if (!name) return;
    try {
      const created = /** @type {GroupWithKeys} */ (
        await api.post('/admin/groups', {
          name,
          keyName: keyInput.value.trim() || undefined,
        })
      );
      revealNext = created.keys[0]?.id ?? -1;
      form.reset();
      ok('已创建');
      await load();
    } catch (err) {
      fail(err);
    }
  }
}
