/**
 * 主题三态：跟随系统 / 强制亮 / 强制暗。
 *
 * 为什么单独一个文件、还必须同步加载在 <head> 里：
 * 属性得在首次绘制前写上，否则选了强制暗色的人每次进页面都会闪一下白底。
 * CSP 是 script-src 'self' 且不给 unsafe-inline，内联 <script> 会被挡，
 * 所以只能是外部文件；又不能用 type="module"（那会 defer 到解析完才跑，
 * 照样闪）。所以这里是 classic script，靠 window 上的全局跟 app.js 通信。
 *
 * 具体配色见 app.css：那边所有颜色都是 light-dark(亮, 暗)，
 * 这里只负责改 color-scheme 的来源，不碰任何具体颜色。
 */
(() => {
  const KEY = 'sandbox-pay:theme';
  const root = document.documentElement;

  /** 只认这三个值，localStorage 里是别的（手改过、旧版本残留）就当没设过。 */
  const isValid = (v) => v === 'light' || v === 'dark' || v === 'system';

  function read() {
    try {
      const v = localStorage.getItem(KEY);
      return isValid(v) ? v : 'system';
    } catch {
      // 隐私模式下 localStorage 可能直接抛，这时退回跟随系统
      return 'system';
    }
  }

  /**
   * system 是删属性而不是写 data-theme="system"。
   * 这样 CSS 里判断「当前实际是暗色」只有两种情况（见 app.css 里日期图标那条），
   * 多一个值就多一个分支。
   */
  function apply(mode) {
    if (mode === 'system') root.removeAttribute('data-theme');
    else root.setAttribute('data-theme', mode);
  }

  // 立刻执行 —— 这是整个文件要同步加载的唯一原因
  apply(read());

  const listeners = new Set();
  const osDark = matchMedia('(prefers-color-scheme: dark)');

  /** 当前真正渲染出来的是亮还是暗（把 system 解析成具体值）。 */
  function effective(mode = read()) {
    if (mode === 'system') return osDark.matches ? 'dark' : 'light';
    return mode;
  }

  function set(mode) {
    if (!isValid(mode)) return;
    try {
      localStorage.setItem(KEY, mode);
    } catch {
      // 存不了就只对本次会话生效，不影响切换本身
    }
    apply(mode);
    for (const fn of listeners) fn(mode, effective(mode));
  }

  // 跟随系统时，系统切换要通知 UI 更新图标（属性没变，CSS 会自己翻）
  osDark.addEventListener('change', () => {
    if (read() !== 'system') return;
    for (const fn of listeners) fn('system', effective('system'));
  });

  window.__theme = {
    get: read,
    effective,
    set,
    /** 亮 → 暗 → 跟随系统 → 亮，循环。 */
    cycle() {
      const order = ['light', 'dark', 'system'];
      const next = order[(order.indexOf(read()) + 1) % order.length];
      set(next);
      return next;
    },
    /** 注册变更回调，返回取消函数。 */
    onChange(fn) {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
  };
})();
