// Classic script: set the theme synchronously before the first paint.
(() => {
  const key = 'sandbox-pay:theme';
  const root = document.documentElement;
  const valid = (value: unknown): value is ThemeMode =>
    value === 'light' || value === 'dark' || value === 'system';
  function stored(): ThemeMode {
    try {
      const value = localStorage.getItem(key);
      return valid(value) ? value : 'system';
    } catch {
      return 'system';
    }
  }
  let mode = stored();
  function apply(value: ThemeMode) {
    if (value === 'system') root.removeAttribute('data-theme');
    else root.setAttribute('data-theme', value);
  }
  apply(mode);
  const listeners = new Set<(mode: ThemeMode, effective: EffectiveTheme) => void>();
  const osDark = matchMedia('(prefers-color-scheme: dark)');
  const effective = (value: ThemeMode = mode): EffectiveTheme =>
    value === 'system' ? (osDark.matches ? 'dark' : 'light') : value;
  function notify() {
    for (const listener of listeners) listener(mode, effective());
  }
  function set(value: ThemeMode) {
    if (!valid(value)) return;
    mode = value;
    try { localStorage.setItem(key, value); } catch { /* session preference still applies */ }
    apply(value);
    notify();
  }
  osDark.addEventListener('change', () => { if (mode === 'system') notify(); });
  window.addEventListener('storage', (event) => {
    if (event.key !== key && event.key !== null) return;
    mode = stored();
    apply(mode);
    notify();
  });
  window.__theme = {
    get: () => mode,
    effective,
    set,
    cycle() {
      const order: ThemeMode[] = ['light', 'dark', 'system'];
      const next = order[(order.indexOf(mode) + 1) % order.length];
      set(next);
      return next;
    },
    onChange(listener) {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
  };
})();
