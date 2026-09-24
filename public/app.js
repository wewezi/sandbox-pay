/* Sandbox Pay — 全站共用脚本：api 封装 / 登录态 / toast / 分页 / 顶栏。
   ES module，各页面 import 用得到的部分。

   类型靠 JSDoc 标注，由 public/tsconfig.json 的 checkJs 检查（npx tsc -p public --noEmit）。
   注释在浏览器里不存在，所以**没有构建步骤** —— 这个文件就是浏览器加载的原文。
   接口返回的形状写在 public/api-types.d.ts（独立声明，后端是 Go、引不进来）。 */

const BASE = '/v1/api';
const TOKEN_KEY = 'token';

export const getToken = () => localStorage.getItem(TOKEN_KEY);
/** @param {string} t */
export const setToken = (t) => localStorage.setItem(TOKEN_KEY, t);
export const clearToken = () => localStorage.removeItem(TOKEN_KEY);

/**
 * 后端返回 { code, msg, data }，code !== 200 一律当失败。401 清 token 跳登录。
 *
 * @param {'GET' | 'POST' | 'DELETE'} method
 * @param {string} path
 * @param {unknown} [body]
 * @param {{ signal?: AbortSignal }} [opts]
 * @returns {Promise<unknown>} data 字段，调用方自己收窄
 */
async function request(method, path, body, opts = {}) {
  /** @type {Record<string, string>} */
  const headers = {};
  const token = getToken();
  if (token) headers['Authorization'] = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  let res;
  try {
    res = await fetch(BASE + path, {
      method,
      headers,
      credentials: 'include',
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: opts.signal,
    });
  } catch (err) {
    // abort 不是「网络失败」，原样抛出去让调用方按取消处理
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new Error('网络请求失败，检查服务是否在运行');
  }

  // 登录页上的 401 是「用户名或密码错误」，不是 token 过期 ——
  // 那里不跳转也不套用过期文案，让下面的分支把服务端原文抛出来。
  if (res.status === 401 && !location.pathname.endsWith('/login.html')) {
    clearToken();
    location.href =
      '/login.html?redirect=' +
      encodeURIComponent(location.pathname + location.search);
    throw new Error('登录已过期');
  }

  /** @type {{ code?: unknown, data?: unknown, msg?: unknown, message?: unknown } | null} */
  let json = null;
  try {
    // res.json() 是 any，先落到 unknown 再判形状 ——
    // 直接赋给上面那个类型等于凭空保证服务端一定按 envelope 返回。
    const parsed = /** @type {unknown} */ (await res.json());
    if (typeof parsed === 'object' && parsed !== null) json = parsed;
  } catch {
    /* 空响应体或非 JSON，下面按状态码兜底 */
  }

  if (!res.ok) {
    throw new Error(pickMessage(json) || `请求失败 (${res.status})`);
  }
  if (json && typeof json === 'object' && 'code' in json && json.code !== 200) {
    throw new Error(pickMessage(json) || '请求失败');
  }
  return json ? json.data : null;
}

/**
 * 后端错误信息可能落在 msg / message，message 还可能是数组（多字段校验失败）。
 *
 * @param {{ msg?: unknown, message?: unknown } | null} json
 * @returns {string}
 */
function pickMessage(json) {
  if (!json || typeof json !== 'object') return '';
  const m = json.message ?? json.msg;
  if (Array.isArray(m)) return m.join('; ');
  if (typeof m === 'string' && m !== 'success') return m;
  return '';
}

/*
 * 三个方法都返回 unknown —— 后端 envelope 的 data 是什么形状只有调用方知道。
 * 调用处自己用行内 type 断言收窄（见 groups.js 的 load），
 * 别在这里假装知道类型。
 *
 * 注意这里是普通块注释而不是 JSDoc：里面举的例子含 @type，
 * 写成 /** 会被 tsc 当成真的类型标注去解析。
 *
 * signal 给轮询用：页面卸载时 abort，请求跟着取消。
 */
export const api = {
  /** @param {string} path @param {{ signal?: AbortSignal }} [opts] */
  get: (path, opts) => request('GET', path, undefined, opts),
  /** @param {string} path @param {unknown} [body] @param {{ signal?: AbortSignal }} [opts] */
  post: (path, body, opts) => request('POST', path, body ?? {}, opts),
  /** @param {string} path @param {{ signal?: AbortSignal }} [opts] */
  del: (path, opts) => request('DELETE', path, undefined, opts),
};

/**
 * 是不是「请求被取消」。
 *
 * abort 不是错误，调用方收到它应当静默返回 —— 别弹 toast，
 * 也别往可能已经拆掉的 DOM 上写东西。
 *
 * @param {unknown} err
 * @returns {boolean}
 */
export const isAbort = (err) =>
  err instanceof DOMException && err.name === 'AbortError';

/**
 * 「后发先至」防护：返回一个每次调用都会掐掉上一次的 signal 工厂。
 *
 * 列表页的问题是快速连点「查询」会并发好几个请求，先发的响应后到就会
 * 盖掉后发的结果 —— 渲染出跟当前筛选条件不符的列表。每次开新请求前
 * abort 掉上一个，被取消的那个会抛 AbortError，由 isAbort 静默吃掉。
 *
 * @returns {() => AbortSignal}
 */
export function latest() {
  /** @type {AbortController | null} */
  let current = null;
  return () => {
    current?.abort();
    current = new AbortController();
    return current.signal;
  };
}

/**
 * 未登录直接跳登录页；返回 false 表示调用方应停止初始化。
 * @returns {boolean}
 */
export function requireAuth() {
  if (getToken()) return true;
  location.href =
    '/login.html?redirect=' +
    encodeURIComponent(location.pathname + location.search);
  return false;
}

export async function logout() {
  try {
    await api.post('/auth/logout');
  } catch {
    /* cookie 清不掉也无所谓，本地 token 清了就登不进去 */
  }
  clearToken();
  location.href = '/login.html';
}

// ── toast ──────────────────────────────────────

/**
 * @param {string} msg
 * @param {'' | 'ok' | 'err'} [kind]
 */
export function toast(msg, kind = '') {
  let box = document.getElementById('toasts');
  if (!box) {
    box = document.createElement('div');
    box.id = 'toasts';
    document.body.appendChild(box);
  }
  const node = document.createElement('div');
  node.className = 'toast' + (kind ? ' ' + kind : '');
  node.textContent = msg;
  box.appendChild(node);
  setTimeout(() => node.remove(), 3200);
}

/** @param {string} msg */
export const ok = (msg) => toast(msg, 'ok');
/** @param {unknown} err */
export const fail = (err) =>
  toast(err instanceof Error ? err.message : String(err), 'err');

// ── DOM 小工具 ─────────────────────────────────

/**
 * 泛型转发给 querySelector，`$('#foo')` 这种已知标签能直接推出元素类型。
 *
 * @template {Element} [T=Element]
 * @param {string} sel
 * @returns {T | null}
 */
export const $ = (sel) => /** @type {T | null} */ (document.querySelector(sel));

/**
 * @template {Element} [T=Element]
 * @param {string} sel
 * @returns {T[]}
 */
export const $$ = (sel) =>
  /** @type {T[]} */ (Array.from(document.querySelectorAll(sel)));

/**
 * 找元素，找不到就抛 —— 页面结构写错时立刻炸在这里，
 * 而不是后面某处安静地对 null 取属性。
 *
 * 返回 HTMLElement 而不是 Element：调用方基本都要碰 dataset / style /
 * classList，给 Element 每次都得再断言一次。要更具体的类型用 mustBe。
 *
 * @param {string} sel
 * @param {ParentNode} [root]
 * @returns {HTMLElement}
 */
export function must(sel, root = document) {
  const found = root.querySelector(sel);
  if (!found) throw new Error(`找不到元素：${sel}`);
  if (!(found instanceof HTMLElement)) {
    throw new Error(`${sel} 不是 HTMLElement`);
  }
  return found;
}

/**
 * 要具体元素类型时用这个，带运行时 instanceof 检查。
 *
 * 比行内类型断言强：那种写法是空口保证，模版里把 input 改成 textarea，
 * tsc 照样过，运行时读 .value 才发现不对。这里当场抛。
 *
 * @template {HTMLElement} T
 * @param {string} sel
 * @param {new () => T} Ctor 期望的构造函数，比如 HTMLInputElement
 * @param {ParentNode} [root]
 * @returns {T}
 */
export function mustBe(sel, Ctor, root = document) {
  const found = must(sel, root);
  if (!(found instanceof Ctor)) {
    throw new Error(`${sel} 不是 ${Ctor.name}`);
  }
  return found;
}

/**
 * 克隆 <template> 的内容。模版写在 HTML 里、JS 只管填字段，
 * 比 el() 手拼 DOM 树短得多，而且天然符合 CSP（没有 innerHTML）。
 *
 * 必须用 importNode 而不是 tpl.content.cloneNode(true) —— 这一条踩过：
 *
 * template.content 属于一个「惰性文档」，那个文档没有自定义元素注册表。
 * cloneNode 出来的节点还挂在那个文档上，所以里面的 <sp-xxx> 不会升级，
 * customElements.upgrade() 对它也是空操作（元素得先属于主文档）。
 * 未升级的元素上赋属性（`card.group = m`）只是建了个实例自有属性，
 * 等它插进文档真升级时，这个自有属性正好盖住原型上的 setter ——
 * setter 永远不执行，组件收不到数据，而且不报错。
 *
 * importNode 会把节点收养进主文档，升级当场就做完了。
 *
 * @param {string} id <template> 的 id
 * @returns {DocumentFragment}
 */
export function fromTemplate(id) {
  const tpl = document.getElementById(id);
  if (!(tpl instanceof HTMLTemplateElement)) {
    throw new Error(`找不到 <template id="${id}">`);
  }
  return /** @type {DocumentFragment} */ (
    document.importNode(tpl.content, true)
  );
}

/**
 * el() 的 prop 值。
 *
 * 刻意不写成 unknown：里面要 String(v) 转成属性值，而 unknown 允许传对象，
 * 那样会安静地写成 "[object Object]"。写死这几种，传错当场类型报错。
 *
 * @typedef {string | number | boolean | EventListener | null | undefined} ElProp
 */

/**
 * 创建元素。textContent 赋值，绝不拼 innerHTML，省掉转义问题。
 *
 * 共用层自己还在用（toast / 顶栏 / 确认弹窗），页面已经不调了。
 *
 * @param {string} tag
 * @param {Record<string, ElProp>} [props]
 * @param {(Node | string | null | undefined)[] | Node | string} [children]
 * @returns {HTMLElement}
 */
export function el(tag, props = {}, children = []) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (v === undefined || v === null) continue;
    if (typeof v === 'function') {
      // onclick / oninput …… → addEventListener('click' | 'input', fn)
      if (k.startsWith('on'))
        node.addEventListener(k.slice(2).toLowerCase(), v);
      continue;
    }
    if (k === 'class') node.className = String(v);
    else if (k === 'text') node.textContent = String(v);
    else node.setAttribute(k, String(v));
  }
  for (const c of Array.isArray(children) ? children : [children]) {
    if (c)
      node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return node;
}

/**
 * 表格单元格。空值显示破折号。
 *
 * text 收在这几种而不是 unknown：见 ElProp 的说明，
 * 传对象进来会变成 "[object Object]" 而不是报错。
 *
 * @param {string | number | null | undefined} text
 * @param {string} [cls]
 */
export function td(text, cls) {
  return el('td', {
    class: cls,
    text:
      text === null || text === undefined || text === '' ? '—' : String(text),
  });
}

// ── 主题切换 ───────────────────────────────────

/** @type {Record<ThemeMode, string>} */
const THEME_LABEL = { light: '亮色', dark: '暗色', system: '跟随系统' };

/**
 * 主题切换按钮。点一下循环 亮 → 暗 → 跟随系统。
 *
 * 三态而不是两态：只给亮/暗的话，用户一旦点过就永远脱离系统设置，
 * 想回去只能清 localStorage。
 *
 * showLabel 默认开：图标含义是变的（太阳/月亮/显示器），侧边栏里其它项
 * 都带文字，纯图标反而要靠 hover 才看得懂。浮动那份没这个空间，关掉。
 *
 * 状态和持久化都在 theme.js（它必须同步跑在 <head> 里，见那边的说明），
 * 这里只管画按钮。theme.js 没加载成功就返回 null，页面其它部分不受影响。
 */
/**
 * @param {{ showLabel?: boolean }} [opts]
 * @returns {HTMLElement | null} theme.js 没加载成功时返回 null
 */
export function themeToggle({ showLabel = true } = {}) {
  const theme = window.__theme;
  if (!theme) return null;

  const btn = el('button', {
    class: 'theme-toggle link',
    type: 'button',
    onclick: () => theme.cycle(),
  });

  // 图标靠 CSS 按 data-mode 选，这里只写属性和文字
  /** @param {ThemeMode} mode */
  const sync = (mode) => {
    btn.dataset.mode = mode;
    const label = `主题：${THEME_LABEL[mode]}`;
    btn.setAttribute('aria-label', label);
    btn.setAttribute('title', label);
    // textContent 不影响 ::before 的图标，只换文本节点
    if (showLabel) btn.textContent = THEME_LABEL[mode];
  };

  sync(theme.get());
  theme.onChange(sync);
  return btn;
}

// ── 顶栏 ───────────────────────────────────────

/** @type {readonly [href: string, label: string][]} */
const NAV = [
  ['/orders.html', '订单'],
  ['/notifications.html', '通知队列'],
  ['/groups.html', '分组与 Key'],
  ['/sandbox.html', '沙箱'],
];

/**
 * 渲染顶栏到 #topbar，current 传当前页路径以高亮。
 * @param {string} current
 */
export function renderTopbar(current) {
  const host = document.getElementById('topbar');
  if (!host) return;
  host.className = 'topbar';
  host.appendChild(el('div', { class: 'brand', text: 'Sandbox Pay' }));
  host.appendChild(
    el(
      'nav',
      {},
      NAV.map(([href, label]) =>
        el('a', { href, text: label, class: href === current ? 'active' : '' }),
      ),
    ),
  );
  host.appendChild(el('div', { class: 'spacer' }));
  const toggle = themeToggle();
  if (toggle) host.appendChild(toggle);
  host.appendChild(
    el('button', { class: 'link', text: '退出', onclick: () => void logout() }),
  );
}

/**
 * 给没有顶栏的页面用（login / cashier）：固定在右上角。
 *
 * login 也要能切 —— 否则系统是亮色、偏好暗色的人得先登进去才换得了。
 */
export function mountFloatingThemeToggle() {
  const toggle = themeToggle({ showLabel: false });
  if (!toggle) return;
  toggle.classList.add('floating');
  document.body.appendChild(toggle);
}

// ── 格式化 ─────────────────────────────────────

/**
 * 金额。后端给的已经是元（分 → 元的换算在 API 边界上做完了），这里只管显示。
 *
 * 用 Intl 而不是 toFixed(2) 是为了千分位分组 —— 后台要看六位数的金额，
 * `￥1,234,567.89` 比 `￥1234567.89` 好数一个数量级。
 *
 * 但符号还是自己拼的全角 ￥，没用 style:'currency'：
 * 那个会输出半角 ¥，跟这个项目原来的显示不一致。分组要，符号不动。
 */
const YUAN_FMT = new Intl.NumberFormat('zh-CN', {
  style: 'decimal',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

/** @param {unknown} n */
export const yuan = (n) => '￥' + YUAN_FMT.format(Number(n) || 0);

/**
 * 固定 zh-CN + 24 小时制，不跟随浏览器 locale ——
 * 后台是给自己看的，日志和列表里的时间格式要能对得上。
 * hourCycle 显式给 h23，否则 zh-CN 在某些实现里会出现 "24:00"。
 */
const DATETIME_FMT = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

/**
 * @param {string | number | Date | null | undefined} v
 * @returns {string} 空值给空串，解析不了的原样返回
 */
export function dateTime(v) {
  if (!v) return '';
  const d = new Date(v);
  if (isNaN(d.getTime())) return String(v);
  // zh-CN 给的是 "2026/09/12 13:05:00"，统一成短横线好和后端日志对齐
  return DATETIME_FMT.format(d).replace(/\//g, '-');
}

/**
 * 成功率。传入 0..1 的比例，不是百分数。
 *
 * 用 style:'percent' 而不是自己 `*100 + '%'`：小数位和百分号的位置
 * 交给 Intl 按 locale 决定。
 */
const PERCENT_FMT = new Intl.NumberFormat('zh-CN', {
  style: 'percent',
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
});

/** @param {number} ratio 0..1 */
export const percent = (ratio) => PERCENT_FMT.format(ratio);

/**
 * 笔数。带千分位 —— 六位数的笔数要能一眼看出数量级。
 */
const COUNT_FMT = new Intl.NumberFormat('zh-CN');

/** @param {number} n */
export const count = (n) => COUNT_FMT.format(n);

/**
 * 相对时间。
 *
 * 交给 Intl.RelativeTimeFormat 而不是自己拼 `${n} 分钟前` ——
 * 单复数、词序、「昨天」这类特殊说法都是 locale 数据里的事。
 *
 * numeric:'auto' 会把 -1 天说成「昨天」、0 秒说成「现在」。
 * 但「昨天」得真的是昨天：24 小时以内一律按经过时间说（凌晨 1 点看
 * 两小时前的单，该是「2小时前」而不是「昨天」），超过一天才切到日历天。
 */
const REL_FMT = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' });

/** 当天零点的时间戳，用来算日历天差 */
const midnight = (/** @type {Date} */ d) =>
  new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

/**
 * @param {string | number | Date | null | undefined} v
 * @param {Date} [now] 注入用，测试和批量渲染时固定基准
 * @returns {string} 空值或解析不了给空串
 */
export function relTime(v, now = new Date()) {
  if (!v) return '';
  const then = new Date(v);
  if (isNaN(then.getTime())) return '';

  const secs = Math.round((then.getTime() - now.getTime()) / 1000);
  const abs = Math.abs(secs);

  if (abs < 60) return REL_FMT.format(secs, 'second');
  if (abs < 3600) return REL_FMT.format(Math.round(secs / 60), 'minute');
  if (abs < 86400) return REL_FMT.format(Math.round(secs / 3600), 'hour');

  const days = Math.round((midnight(then) - midnight(now)) / 86400000);
  return REL_FMT.format(days, 'day');
}

/**
 * 秒数 → MM:SS，≤0 返回 null 交给调用方显示「已过期」。
 *
 * @param {number} seconds
 * @returns {string | null}
 */
export function mmss(seconds) {
  if (seconds <= 0) return null;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

// ── 分页 ───────────────────────────────────────

/**
 * @typedef {object} PageState
 * @property {number} page
 * @property {number} pageSize
 * @property {number} total
 */

/**
 * 渲染分页条。
 *
 * @param {HTMLElement} host 容器元素
 * @param {PageState} state
 * @param {(page: number) => void} onGo 翻页回调，参数为目标页码
 */
export function renderPager(host, state, onGo) {
  const { page, pageSize, total } = state;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  host.className = 'pager';

  const prev = el('button', { text: '上一页', onclick: () => onGo(page - 1) });
  const next = el('button', { text: '下一页', onclick: () => onGo(page + 1) });
  if (prev instanceof HTMLButtonElement) prev.disabled = page <= 1;
  if (next instanceof HTMLButtonElement) next.disabled = page >= pages;

  // replaceChildren 一次换掉，省掉「先 textContent='' 再逐个 append」
  host.replaceChildren(
    el('span', {
      class: 'info',
      text: `共 ${total} 条，第 ${page} / ${pages} 页`,
    }),
    prev,
    next,
  );
}

/**
 * 把对象拼成 query string，跳过空值。
 *
 * @param {Record<string, string | number | undefined | null>} params
 * @returns {string} 带 ? 前缀，全空时返回空串
 */
export function qs(params) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') p.set(k, String(v));
  }
  const s = p.toString();
  return s ? '?' + s : '';
}

// ── 确认弹窗 ───────────────────────────────────

/**
 * 把一个 <dialog class="modal"> 挂到 body 上并 showModal，返回关闭函数。
 *
 * 用 showModal() 白拿三件事，原来是手写 .mask 自己实现的：
 *   焦点陷阱   —— Tab 出不去弹窗
 *   Esc 关闭   —— 走 cancel / close 事件
 *   背景遮罩   —— ::backdrop 伪元素，不用自己塞一层 div
 * 还有 inert：模态期间背景内容自动不可交互，屏幕阅读器也读不到。
 *
 * confirmDialog 和 orders 的退款弹窗共用这一套，免得各自实现一遍
 * 「Esc 关掉了但节点还留在 DOM 里」那种漏。
 *
 * @param {HTMLDialogElement} dialog
 * @returns {() => void} 关闭并从 DOM 摘掉
 */
export function openModal(dialog) {
  document.body.appendChild(dialog);

  const dismiss = () => {
    if (dialog.open) dialog.close();
    dialog.remove();
  };

  // Esc 走 cancel。不 preventDefault，让原生关闭流程照走，
  // 再在 close 上把节点摘掉 —— 只听 cancel 的话点遮罩关不掉节点。
  dialog.addEventListener('close', () => dialog.remove(), { once: true });
  dialog.addEventListener('click', (e) => {
    if (e.target === dialog) dismiss();
  });

  dialog.showModal();
  return dismiss;
}

/**
 * 返回 Promise<boolean>，走原生 <dialog> 而不是 window.confirm，样式统一。
 *
 * @param {object} opts
 * @param {string} opts.title
 * @param {string} opts.message
 * @param {string} [opts.confirmText]
 * @param {boolean} [opts.danger] 危险操作用红色确认按钮
 * @returns {Promise<boolean>}
 */
export function confirmDialog({
  title,
  message,
  confirmText = '确定',
  danger = false,
}) {
  return new Promise((resolve) => {
    const dialog = /** @type {HTMLDialogElement} */ (
      el('dialog', { class: 'modal' })
    );

    // close 事件 Esc / 点遮罩都会来。按钮路径先把 settled 钉住，
    // 否则 dismiss → close 会把「确定」改写成「取消」。
    let settled = false;
    /** @type {() => void} */
    let close = () => {};
    /** @param {boolean} result */
    const finish = (result) => {
      if (settled) return;
      settled = true;
      close();
      resolve(result);
    };

    const confirm = el('button', {
      class: danger ? 'danger' : 'primary',
      text: confirmText,
      onclick: () => finish(true),
    });

    dialog.append(
      el('h3', { text: title }),
      el('p', { class: 'muted', text: message }),
      el('div', { class: 'acts' }, [
        el('button', { text: '取消', onclick: () => finish(false) }),
        confirm,
      ]),
    );

    dialog.addEventListener('close', () => finish(false));

    close = openModal(dialog);
    confirm.focus();
  });
}

// ── <sp-time> ──────────────────────────────────

/**
 * 活的相对时间。用法：
 *
 * ```html
 * <sp-time datetime="2026-09-12T07:39:58.000Z"></sp-time>
 * ```
 *
 * 为什么要这么一个元素，而不是渲染时算一次字符串：
 *
 * - **它自己会走**。列表页开着不动，「1分钟前」到点变成「2分钟前」，
 *   不用整页重刷。原来这个字符串是后端拼的，等于响应那一刻的快照。
 * - **时区和时钟按浏览器算**。后端拼的话用的是服务器时钟。
 * - **语义标签**。内部渲染成 `<time datetime>`，机器可读，
 *   title 里放完整时间给人看。
 *
 * 所有实例共用一个 interval：第一个连上时起，最后一个断开时停 ——
 * 一页 50 行不该有 50 个定时器。
 */

/** @type {Set<SpTime>} */
const liveTimes = new Set();
/** @type {ReturnType<typeof setInterval> | null} */
let timeTicker = null;

/** 30s 一跳：秒级只在第一分钟内有意义，再密就是白耗电。 */
const TICK_MS = 30_000;

function tickAll() {
  const now = new Date();
  for (const t of liveTimes) t.render(now);
}

function startTicker() {
  if (timeTicker) return;
  timeTicker = setInterval(tickAll, TICK_MS);
  // 后台标签页的定时器会被节流甚至冻结，回到前台先补一次，
  // 否则切回来看到的是离开时的旧文案。
  document.addEventListener('visibilitychange', onVisible);
}

function stopTicker() {
  if (!timeTicker) return;
  clearInterval(timeTicker);
  timeTicker = null;
  document.removeEventListener('visibilitychange', onVisible);
}

function onVisible() {
  if (!document.hidden) tickAll();
}

class SpTime extends HTMLElement {
  static observedAttributes = ['datetime'];

  /** @type {HTMLTimeElement | null} */
  #time = null;

  connectedCallback() {
    if (!this.#time) {
      this.#time = document.createElement('time');
      this.appendChild(this.#time);
    }
    liveTimes.add(this);
    startTicker();
    this.render();
  }

  disconnectedCallback() {
    liveTimes.delete(this);
    // 最后一个走了就把定时器收掉，别留一个空转的 interval
    if (liveTimes.size === 0) stopTicker();
  }

  attributeChangedCallback() {
    if (this.isConnected) this.render();
  }

  /** @param {Date} [now] 批量渲染时传同一个基准，省掉每行 new Date() */
  render(now = new Date()) {
    const raw = this.getAttribute('datetime');
    if (!this.#time) return;
    this.#time.dateTime = raw ?? '';
    this.#time.textContent = relTime(raw, now) || '—';
    // 相对时间说不清具体哪天，完整时间放 title
    const exact = dateTime(raw);
    if (exact) this.#time.title = exact;
    else this.#time.removeAttribute('title');
  }
}

customElements.define('sp-time', SpTime);

/**
 * 造一个 `<sp-time>`。
 *
 * @param {string | number | Date | null | undefined} v
 * @returns {HTMLElement}
 */
export function timeEl(v) {
  const node = document.createElement('sp-time');
  if (v) node.setAttribute('datetime', new Date(v).toISOString());
  return node;
}
