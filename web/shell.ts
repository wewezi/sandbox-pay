import { css, html, LitElement, nothing } from 'lit';
import { keyed } from 'lit/directives/keyed.js';
import { getToken } from './app';
import { appRoutes, navigate, subscribeRoutes } from './services/router';
import { sharedStyles } from './styles/shared';

class SpApp extends LitElement {
  static override properties = {
    path: { state: true }, admin: { state: true }, loading: { state: true }, loadError: { state: true },
    view: { state: true }, sessionToken: { state: true },
  };
  static override styles = [sharedStyles, css`
    :host { display: block; min-height: 100svh; }
    .route-outlet { min-width: 0; }
    .route-message { padding: var(--s8); min-height: 240px; display: flex; flex-direction: column; align-items: start; gap: var(--s4); }
    .route-message.admin { margin-left: var(--sidebar-w); }
    .route-message h1 { font-size: 20px; margin: 0; }
    @media (max-width: 720px) {
      /* Keep the last page controls above the floating navigation and home indicator. */
      .route-outlet.admin { padding-bottom: calc(80px + env(safe-area-inset-bottom, 0px)); }
      .route-message { padding: var(--s4); }
      .route-message.admin { margin-left: 0; }
    }
  `];
  declare private path: string;
  declare private admin: boolean;
  declare private loading: boolean;
  declare private loadError: boolean;
  declare private view: HTMLElement | null;
  declare private sessionToken: string | null;
  private generation = 0;
  private viewAddress = '';
  private unsubscribe?: () => void;
  private scrollFrame?: number;
  private scrollObserver?: ResizeObserver;
  private scrollTimeout?: number;
  private readonly interruptScrollRestore = () => this.cancelScrollRestore();
  private readonly accountChanged = (event: StorageEvent) => {
    if (event.key === 'token' || event.key === null) void this.activate({ kind: 'replace' }, true);
  };

  constructor() {
    super();
    this.path = location.pathname;
    this.admin = false;
    this.loading = true;
    this.loadError = false;
    this.view = null;
    this.sessionToken = getToken();
  }

  override connectedCallback() {
    super.connectedCallback();
    window.addEventListener('storage', this.accountChanged);
    this.unsubscribe = subscribeRoutes(change => { void this.activate(change); });
  }

  override disconnectedCallback() {
    this.generation++;
    this.view?.remove();
    this.view = null;
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.cancelScrollRestore();
    window.removeEventListener('storage', this.accountChanged);
    super.disconnectedCallback();
  }

  private async activate(change: { kind: string; scroll?: [number, number] }, force = false) {
    const path = location.pathname;
    if (path === '/') {
      navigate(getToken() ? '/dashboard' : '/login', { replace: true });
      return;
    }
    const route = appRoutes[path];
    if (route?.admin && !getToken()) {
      navigate('/login?redirect=' + encodeURIComponent(path + location.search + location.hash), { replace: true });
      return;
    }
    const address = path + location.search;
    if (!force && this.view && this.viewAddress === address) {
      this.restoreViewPosition(change);
      return;
    }
    const generation = ++this.generation;
    this.cancelScrollRestore();
    // Resolve pending confirmations before disconnecting the page that owns its action.
    document.querySelectorAll('sp-confirm-dialog').forEach(dialog => dialog.remove());
    this.view?.remove();
    this.view = null;
    this.viewAddress = '';
    this.path = path;
    this.sessionToken = getToken();
    this.admin = route?.admin ?? Boolean(this.sessionToken);
    this.loading = Boolean(route);
    this.loadError = false;
    document.title = (route?.title || '页面不存在') + ' · Mintweft';
    if (!this.admin) document.querySelector('sp-toast-region')?.remove();
    if (!route) return;
    try {
      await route.load();
      if (!this.isConnected || generation !== this.generation) return;
      // A logout in another tab can arrive while the module is loading.
      if (route.admin && !getToken()) {
        navigate('/login?redirect=' + encodeURIComponent(path + location.search + location.hash), { replace: true });
        return;
      }
      const view = document.createElement(route.element);
      if (route.admin) view.setAttribute('admin', '');
      this.view = view;
      this.viewAddress = address;
      this.loading = false;
      await this.updateComplete;
      if (view instanceof LitElement) await view.updateComplete;
      if (!this.isConnected || generation !== this.generation) return;
      if (change.kind !== 'initial') {
        const heading = view.shadowRoot?.querySelector<HTMLElement>('h1');
        if (heading) { heading.tabIndex = -1; heading.focus({ preventScroll: true }); }
      }
      this.restoreViewPosition(change);
    } catch {
      if (!this.isConnected || generation !== this.generation) return;
      this.view?.remove();
      this.view = null;
      this.viewAddress = '';
      this.loading = false;
      this.loadError = true;
    }
  }

  private cancelScrollRestore() {
    if (this.scrollFrame !== undefined) cancelAnimationFrame(this.scrollFrame);
    this.scrollFrame = undefined;
    this.scrollObserver?.disconnect();
    this.scrollObserver = undefined;
    if (this.scrollTimeout !== undefined) clearTimeout(this.scrollTimeout);
    this.scrollTimeout = undefined;
    window.removeEventListener('pointerdown', this.interruptScrollRestore);
    window.removeEventListener('wheel', this.interruptScrollRestore);
    window.removeEventListener('keydown', this.interruptScrollRestore);
  }

  private restoreViewPosition(change: { kind: string; scroll?: [number, number] }) {
    this.cancelScrollRestore();
    const restore = () => {
      if (!this.view?.isConnected) { this.cancelScrollRestore(); return; }
      if (location.hash) {
        let id: string;
        try { id = decodeURIComponent(location.hash.slice(1)); } catch { this.cancelScrollRestore(); return; }
        const target = this.view.shadowRoot?.getElementById(id) || document.getElementById(id);
        if (target) { target.scrollIntoView(); this.cancelScrollRestore(); return; }
      }
      if (change.scroll) {
        window.scrollTo(...change.scroll);
        const maxScroll = Math.max(0, document.documentElement.scrollHeight - innerHeight);
        // A restored list can be shorter until its rows arrive. Retry when its
        // layout grows, instead of permanently accepting the loading state's clamp.
        if (change.kind === 'pop' && change.scroll[1] > maxScroll + 1) return;
      } else if (change.kind !== 'initial') window.scrollTo(0, 0);
      this.cancelScrollRestore();
    };
    this.scrollFrame = requestAnimationFrame(() => {
      this.scrollFrame = undefined;
      if (change.kind === 'pop' && change.scroll && this.view) {
        this.scrollObserver = new ResizeObserver(restore);
        this.scrollObserver.observe(this.view);
        this.scrollTimeout = window.setTimeout(() => this.cancelScrollRestore(), 10000);
        window.addEventListener('pointerdown', this.interruptScrollRestore, { once: true });
        window.addEventListener('wheel', this.interruptScrollRestore, { once: true, passive: true });
        window.addEventListener('keydown', this.interruptScrollRestore, { once: true });
      }
      restore();
    });
  }

  protected override render() {
    return html`
      ${this.admin ? keyed(this.sessionToken, html`<sp-topbar .current=${this.path}></sp-topbar>`) : nothing}
      <div class=${'route-outlet' + (this.admin ? ' admin' : '')} aria-busy=${this.loading} data-route=${this.path}>
        ${this.view || html`<div class=${'route-message' + (this.admin ? ' admin' : '')}>
          ${this.loading ? html`<p class="muted" role="status">正在加载页面…</p>`
            : this.loadError ? html`<h1>页面暂时无法加载</h1><p class="muted">请重新加载后继续。</p><button @click=${() => location.reload()}>重新加载</button>`
              : html`<h1>页面不存在</h1><p class="muted">请检查地址，或返回数据看板。</p><a href="/dashboard">返回数据看板</a>`}
        </div>`}
      </div>`;
  }
}

customElements.define('sp-app', SpApp);
