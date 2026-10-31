import { css, html, LitElement, nothing } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { api, logout } from '../services/api';
import { dateTime, relTime } from '../services/format';
import { mustBe } from '../dom';
import { modalStyles, sharedStyles } from '../styles/shared';
import { navigationStyles, pagerStyles, themeStyles, toastStyles } from '../styles/ui';

const themeLabel: Record<ThemeMode, string> = { light: '亮色', dark: '暗色', system: '跟随系统' };

class SpThemeToggle extends LitElement {
  static override properties = {
    mode: { state: true }, showLabel: { type: Boolean }, floating: { type: Boolean, reflect: true },
  };
  static override styles = [sharedStyles, themeStyles, css`
    :host([floating]) { position: fixed; top: var(--s4); left: var(--s4); z-index: 10; }
  `];
  declare mode: ThemeMode;
  declare showLabel: boolean;
  declare floating: boolean;
  private unsubscribe?: () => void;

  constructor() {
    super();
    this.mode = window.__theme?.get() ?? 'system';
    this.showLabel = true;
    this.floating = false;
  }
  override connectedCallback() {
    super.connectedCallback();
    this.mode = window.__theme?.get() ?? 'system';
    this.unsubscribe = window.__theme?.onChange((mode) => { this.mode = mode; });
  }
  override disconnectedCallback() {
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    super.disconnectedCallback();
  }
  protected override render() {
    if (!window.__theme) return nothing;
    const label = `主题：${themeLabel[this.mode]}`;
    return html`<button class="theme-toggle link" type="button" data-mode=${this.mode}
      aria-label=${label} title=${label} @click=${() => window.__theme?.cycle()}>
      ${this.showLabel ? html`<span class="theme-label">${themeLabel[this.mode]}</span>` : nothing}
    </button>`;
  }
}
customElements.define('sp-theme-toggle', SpThemeToggle);

const navigation = [
  ['/dashboard', '数据看板'], ['/groups', 'API令牌'],
  ['/orders', '订单'], ['/notifications', '使用日志'],
  ['/sandbox', '沙箱'],
  ['/developer', '开发接入'], ['/operations', '运行维护'], ['/team', '团队与账号'],
] as const;
class SpTopbar extends LitElement {
  static override properties = { current: { type: String }, loggingOut: { state: true }, accountRole: { state: true } };
  static override styles = [sharedStyles, navigationStyles];
  declare current: string;
  declare loggingOut: boolean;
  declare private accountRole: string;
  private accountRequest?: AbortController;
  constructor() { super(); this.current = ''; this.loggingOut = false; this.accountRole = ''; }
  override connectedCallback() {
    super.connectedCallback();
    this.accountRequest = new AbortController();
    void api.get<{role:string}>('/auth/session', { signal: this.accountRequest.signal }).then(account=>{if(this.isConnected)this.accountRole=account.role;}).catch(()=>{});
  }
  override disconnectedCallback() {
    this.accountRequest?.abort();
    this.accountRequest = undefined;
    this.accountRole = '';
    super.disconnectedCallback();
  }
  private async signOut() {
    if (this.loggingOut) return;
    this.loggingOut = true;
    try { await logout(); }
    finally { if (this.isConnected) this.loggingOut = false; }
  }
  protected override render() {
    return html`<header class="topbar">
      <div class="brand">Mintweft</div>
      <nav aria-label="后台导航">${navigation.filter(([href])=>href!=='/operations'||this.accountRole==='SuperAdmin').map(([href, label]) => html`
        <a href=${href} class=${href === this.current ? 'active' : ''}
          aria-label=${label} title=${label} aria-current=${href === this.current ? 'page' : nothing}>
          <span>${label}</span>
        </a>
      `)}</nav>
      <div class="spacer"></div>
      <sp-theme-toggle></sp-theme-toggle>
      <button class="link" ?disabled=${this.loggingOut} @click=${this.signOut}>
        ${this.loggingOut ? '退出中…' : '退出'}
      </button>
    </header>`;
  }
}
customElements.define('sp-topbar', SpTopbar);

export type PageState = { page: number; pageSize: number; total: number };
class SpPager extends LitElement {
  static override properties = {
    page: { type: Number }, pageSize: { type: Number }, total: { type: Number }, busy: { type: Boolean },
  };
  static override styles = [sharedStyles, pagerStyles];
  declare page: number;
  declare pageSize: number;
  declare total: number;
  declare busy: boolean;
  constructor() { super(); this.page = 1; this.pageSize = 20; this.total = 0; this.busy = false; }
  private go(page: number) {
    const pages = Math.max(1, Math.ceil(this.total / this.pageSize));
    if (this.busy || page < 1 || page > pages || page === this.page) return;
    this.dispatchEvent(new CustomEvent('page-change', { detail: { page }, bubbles: true, composed: true }));
  }
  protected override render() {
    const pages = Math.max(1, Math.ceil(this.total / this.pageSize));
    return html`<div class="pager" aria-label="分页">
      <span class="info">共 ${this.total} 条，第 ${this.page} / ${pages} 页</span>
      <button ?disabled=${this.busy || this.page <= 1} @click=${() => this.go(this.page - 1)}>上一页</button>
      <button ?disabled=${this.busy || this.page >= pages} @click=${() => this.go(this.page + 1)}>下一页</button>
    </div>`;
  }
}
customElements.define('sp-pager', SpPager);

const liveTimes = new Set<SpTime>();
let timeTicker: number | null = null;
function tickTimes() { const now = new Date(); for (const element of liveTimes) element.now = now; }
function onVisible() { if (!document.hidden) tickTimes(); }
class SpTime extends LitElement {
  static override properties = { datetime: { type: String }, now: { state: true } };
  static override styles = css`:host { display: inline; }`;
  declare datetime: string;
  declare now: Date;
  constructor() { super(); this.datetime = ''; this.now = new Date(); }
  override connectedCallback() {
    super.connectedCallback();
    this.now = new Date();
    liveTimes.add(this);
    if (timeTicker === null) {
      timeTicker = window.setInterval(tickTimes, 30_000);
      document.addEventListener('visibilitychange', onVisible);
    }
  }
  override disconnectedCallback() {
    liveTimes.delete(this);
    if (liveTimes.size === 0 && timeTicker !== null) {
      window.clearInterval(timeTicker);
      timeTicker = null;
      document.removeEventListener('visibilitychange', onVisible);
    }
    super.disconnectedCallback();
  }
  protected override render() {
    return html`<time datetime=${this.datetime} title=${dateTime(this.datetime)}>
      ${relTime(this.datetime, this.now) || '—'}
    </time>`;
  }
}
customElements.define('sp-time', SpTime);

type ToastKind = '' | 'ok' | 'err';
type ToastItem = { id: number; message: string; kind: ToastKind };
class SpToastRegion extends LitElement {
  static override properties = { items: { state: true } };
  static override styles = [sharedStyles, toastStyles, css`
    .messages { display: flex; flex-direction: column; gap: var(--s2); }
  `];
  declare items: readonly ToastItem[];
  private sequence = 0;
  private readonly timers = new Map<number, number>();
  constructor() { super(); this.items = []; }
  show(message: string, kind: ToastKind) {
    const id = ++this.sequence;
    this.items = [...this.items, { id, message, kind }];
    this.timers.set(id, window.setTimeout(() => {
      this.items = this.items.filter((item) => item.id !== id);
      this.timers.delete(id);
    }, 3200));
  }
  override disconnectedCallback() {
    for (const timer of this.timers.values()) window.clearTimeout(timer);
    this.timers.clear();
    this.items = [];
    super.disconnectedCallback();
  }
  protected override render() {
    return html`<div class="messages" role="status" aria-live="polite" aria-atomic="false">
      ${repeat(this.items, (item) => item.id,
        (item) => html`<div class=${`toast ${item.kind}`}>${item.message}</div>`)}
    </div>`;
  }
}
customElements.define('sp-toast-region', SpToastRegion);

export function toast(message: string, kind: ToastKind = '') {
  let region = document.querySelector<SpToastRegion>('sp-toast-region');
  if (!region) {
    region = document.createElement('sp-toast-region') as SpToastRegion;
    document.body.appendChild(region);
  }
  region.show(message, kind);
}
export const ok = (message: string) => toast(message, 'ok');
export const fail = (error: unknown) => toast(error instanceof Error ? error.message : String(error), 'err');

export type ConfirmOptions = { title: string; message: string; confirmText?: string; danger?: boolean };
class SpConfirmDialog extends LitElement {
  static override properties = { options: { state: true } };
  static override styles = [sharedStyles, modalStyles];
  declare options: ConfirmOptions;
  private resolve?: (value: boolean) => void;
  constructor() { super(); this.options = { title: '', message: '' }; }
  present(options: ConfirmOptions): Promise<boolean> {
    this.options = options;
    return new Promise((resolve) => {
      this.resolve = resolve;
      void this.updateComplete.then(() => {
        if (!this.isConnected) return this.finish(false);
        const dialog = mustBe('dialog', HTMLDialogElement, this.renderRoot);
        dialog.showModal();
        dialog.querySelector<HTMLButtonElement>('[data-confirm]')?.focus();
      }).catch((error: unknown) => { fail(error); this.finish(false); });
    });
  }
  private finish(accepted: boolean) {
    const resolve = this.resolve;
    if (!resolve) return;
    this.resolve = undefined;
    const dialog = this.renderRoot.querySelector('dialog');
    if (dialog instanceof HTMLDialogElement && dialog.open) dialog.close();
    this.remove();
    resolve(accepted);
  }
  private backdrop(event: MouseEvent) {
    const dialog = event.currentTarget as HTMLDialogElement;
    if (event.target !== dialog) return;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right ||
      event.clientY < rect.top || event.clientY > rect.bottom) this.finish(false);
  }
  override disconnectedCallback() { this.finish(false); super.disconnectedCallback(); }
  protected override render() {
    const options = this.options;
    return html`<dialog class="modal" aria-label=${options.title}
      @close=${() => this.finish(false)}
      @cancel=${(event: Event) => { event.preventDefault(); this.finish(false); }}
      @click=${this.backdrop}>
      <h3>${options.title}</h3><p class="muted">${options.message}</p>
      <div class="acts">
        <button @click=${() => this.finish(false)}>取消</button>
        <button data-confirm class=${options.danger ? 'danger' : 'primary'}
          @click=${() => this.finish(true)}>${options.confirmText ?? '确定'}</button>
      </div>
    </dialog>`;
  }
}
customElements.define('sp-confirm-dialog', SpConfirmDialog);

export function confirmDialog(options: ConfirmOptions): Promise<boolean> {
  const dialog = document.createElement('sp-confirm-dialog') as SpConfirmDialog;
  document.body.appendChild(dialog);
  return dialog.present(options);
}
