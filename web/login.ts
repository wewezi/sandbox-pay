import { html, LitElement, nothing } from 'lit';
import { api, isAbort, RequestController, setToken } from './app';
import type { LoginResult } from './api-types';
import { sharedStyles } from './styles/shared';
import { loginStyles } from './styles/login';
import { validateForm } from './services/form';
import { isAppRoute, navigate } from './services/router';

function safeRedirect() {
  const raw = new URLSearchParams(location.search).get('redirect');
  if (!raw || !raw.startsWith('/')) return '/dashboard';
  try {
    const target = new URL(raw, location.origin);
    const path = target.pathname;
    if (target.origin !== location.origin || target.username || target.password || path === '/login' || !isAppRoute(path)) return '/dashboard';
    return path + target.search + target.hash;
  } catch { return '/dashboard'; }
}

class SpLoginPage extends LitElement {
  static override properties = {
    busy: { state: true }, passwordVisible: { state: true }, errorMessage: { state: true },
  };
  static override styles = [sharedStyles, loginStyles];
  declare busy: boolean;
  declare passwordVisible: boolean;
  declare errorMessage: string;
  private readonly requests = new RequestController(this, () => {
    this.requests.abort();
    this.busy = false;
  });

  constructor() {
    super();
    this.busy = false;
    this.passwordVisible = false;
    this.errorMessage = '';
  }

  override disconnectedCallback() {
    const password = this.renderRoot.querySelector<HTMLInputElement>('#password');
    if (password) password.value = '';
    this.passwordVisible = false;
    super.disconnectedCallback();
  }

  private async submit(event: SubmitEvent) {
    event.preventDefault();
    if (this.busy) return;
    const form = event.currentTarget as HTMLFormElement;
    this.errorMessage = validateForm(form) || '';
    if (this.errorMessage) return;
    const fields = new FormData(form);
    const signal = this.requests.next('login');
    this.busy = true;
    this.errorMessage = '';
    let navigating = false;
    try {
      const data = await api.post<LoginResult>('/auth/login', {
        username: String(fields.get('username') ?? '').trim(),
        password: String(fields.get('password') ?? ''),
      }, { signal });
      if (signal.aborted || !this.isConnected) return;
      setToken(data.token);
      navigate(safeRedirect(), { replace: true });
      navigating = true;
    } catch (error) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) {
        this.errorMessage = error instanceof Error ? error.message : String(error);
      }
    } finally {
      this.requests.release('login', signal);
      if (!navigating && !signal.aborted && this.isConnected) this.busy = false;
    }
  }

  protected override render() {
    return html`<div class="login-page">
      <div class="theme-slot"><sp-theme-toggle .showLabel=${false}></sp-theme-toggle></div>
      <main class="login-main">
        <div class="login-art" aria-hidden="true">
          <span class="dot-logo"></span>
        </div>
        <section class="login-card" aria-label="登录">
          <div class="brand-lockup">
            <h1>Mintweft</h1>
          </div>
          <form id="form" novalidate aria-busy=${this.busy} @submit=${this.submit} @input=${() => { this.errorMessage = ''; }}>
            <div class="login-field">
              <label for="username">用户名</label>
              <div class=${this.errorMessage ? 'input-shell invalid' : 'input-shell'}>
                <input id="username" name="username" autocomplete="username"
                  autocapitalize="none" spellcheck="false" required minlength="3" maxlength="20"
                  ?disabled=${this.busy} aria-invalid=${this.errorMessage ? 'true' : nothing}
                  aria-describedby=${this.errorMessage ? 'login-error' : nothing}>
              </div>
            </div>
            <div class="login-field">
              <label for="password">密码</label>
              <div class=${this.errorMessage ? 'input-shell invalid' : 'input-shell'}>
                <input id="password" name="password" type=${this.passwordVisible ? 'text' : 'password'}
                  autocomplete="current-password" required minlength="6" maxlength="72"
                  ?disabled=${this.busy} aria-invalid=${this.errorMessage ? 'true' : nothing}
                  aria-describedby=${this.errorMessage ? 'login-error' : nothing}>
                <button class="password-toggle" type="button" ?disabled=${this.busy}
                  aria-label=${this.passwordVisible ? '隐藏密码' : '显示密码'}
                  title=${this.passwordVisible ? '隐藏密码' : '显示密码'}
                  @click=${() => { this.passwordVisible = !this.passwordVisible; }}>
                  <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/><path d="m3 3 18 18" ?hidden=${!this.passwordVisible}/></svg>
                </button>
              </div>
            </div>
            ${this.errorMessage ? html`<p class="login-error" id="login-error" role="alert">${this.errorMessage}</p>` : nothing}
            <button id="submit" class="login-submit" type="submit" ?disabled=${this.busy}>
              ${this.busy ? html`<span class="spinner" aria-hidden="true"></span>` : nothing}
              <span>${this.busy ? '登录中…' : '登录'}</span>
            </button>
          </form>
        </section>
      </main>
    </div>`;
  }
}
customElements.define('sp-login-page', SpLoginPage);
