import { LitElement, html, nothing, type PropertyValues } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { api, clearToken, confirmDialog, dateTime, isAbort, ok, requireAuth, type SpSelect } from './app';
import { RequestController } from './controllers/requests';
import { validateForm } from './services/form';
import { navigate } from './services/router';
import { modalStyles, pageStyles, sharedStyles } from './styles/shared';
import { teamStyles } from './styles/team';

type User = { id: number; username: string; role: string; isActive: boolean; groupIds: number[]; createdAt: string };
type Session = { id: string; clientIp: string; userAgent: string; createdAt: string; lastSeenAt: string; expiresAt: string; current: boolean };
type Project = { id: number; name: string };
type Account = { userId: number; username: string; role: string };
type Mode = 'projects' | 'password' | 'role' | null;
type State = {
  account: Account | null; users: User[]; sessions: Session[]; projects: Project[]; authenticated: boolean;
  loading: boolean; busy: string; error: string; createError: string; passwordError: string; modalError: string;
  username: string; password: string; role: string; currentPassword: string; newPassword: string; confirmPassword: string;
  mode: Mode; target: User | null; memberIds: number[]; projectToAdd: string; modalPassword: string; modalRole: string;
};

class SpTeamPage extends LitElement {
  static override styles = [sharedStyles, modalStyles, pageStyles, teamStyles];
  static override properties = { state: { state: true } };
  declare private state: State;
  private readonly requests = new RequestController(this, () => { this.closeModal(); void this.load(); });

  constructor() {
    super();
    this.state = { account: null, users: [], sessions: [], projects: [], authenticated: false, loading: false, busy: '', error: '', createError: '', passwordError: '', modalError: '', username: '', password: '', role: 'Admin', currentPassword: '', newPassword: '', confirmPassword: '', mode: null, target: null, memberIds: [], projectToAdd: '', modalPassword: '', modalRole: 'Admin' };
  }
  private set(values: Partial<State>) { this.state = { ...this.state, ...values }; }
  private get superAdmin() { return this.state.account?.role === 'SuperAdmin'; }
  private passwordValidation(value: string) { return Array.from(value).length < 8 ? '密码至少需要 8 个字符。' : new TextEncoder().encode(value).length > 72 ? '密码过长，请缩短后重试。' : null; }
  override connectedCallback() { super.connectedCallback(); this.set({ authenticated: requireAuth() }); if (this.state.authenticated) void this.load(); }
  override disconnectedCallback() {
    this.set({ mode: null, target: null, password: '', currentPassword: '', newPassword: '', confirmPassword: '', modalPassword: '', busy: '' });
    for (const input of this.renderRoot.querySelectorAll<HTMLInputElement>('input[type=password]')) input.value = '';
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    super.disconnectedCallback();
  }
  protected override updated(changed: PropertyValues) {
    if (!this.isConnected || !changed.has('state')) return;
    const dialog = this.renderRoot.querySelector<HTMLDialogElement>('dialog');
    if (!dialog) return;
    if (this.state.mode && !dialog.open) { dialog.showModal(); dialog.querySelector<HTMLElement>('input, sp-select, [data-close]')?.focus(); }
    else if (!this.state.mode && dialog.open) dialog.close();
  }

  private async load() {
    if (!this.isConnected || !this.state.authenticated) return;
    const signal = this.requests.next('load'); this.set({ loading: true, error: '' });
    try {
      const [account, sessions] = await Promise.all([api.get<Account>('/auth/session', { signal }), api.get<Session[]>('/auth/sessions', { signal })]);
      const [projects, users] = account.role === 'SuperAdmin' ? await Promise.all([api.get<Project[]>('/admin/groups', { signal }), api.get<User[]>('/admin/team/users', { signal })]) : [[], []];
      if (!signal.aborted && this.isConnected) this.set({ account, sessions, projects, users });
    } catch (error) { if (!isAbort(error) && !signal.aborted && this.isConnected) this.set({ error: error instanceof Error ? error.message : String(error) }); }
    finally { if (!signal.aborted && this.isConnected) this.set({ loading: false }); this.requests.release('load', signal); }
  }

  private async perform(key: string, work: (signal: AbortSignal) => Promise<unknown>, message: string, field: 'error' | 'createError' | 'modalError' = 'error') {
    if (!this.isConnected || this.state.busy) return false;
    const signal = this.requests.next('action'); this.set({ busy: key, [field]: '' });
    try {
      await work(signal);
      if (signal.aborted || !this.isConnected) return false;
      ok(message); return true;
    } catch (error) { if (!isAbort(error) && !signal.aborted && this.isConnected) this.set({ [field]: error instanceof Error ? error.message : String(error) }); return false; }
    finally { if (this.isConnected) this.set({ busy: '' }); this.requests.release('action', signal); }
  }

  private async createUser(event: SubmitEvent) {
    event.preventDefault(); const name = this.state.username.trim();
    const usernameError = new TextEncoder().encode(name).length < 3 || new TextEncoder().encode(name).length > 20 || /\s/.test(name) ? '用户名需要 3–20 字节，不能包含空白字符。' : null;
    const error = validateForm(event.currentTarget as HTMLFormElement) ?? usernameError ?? this.passwordValidation(this.state.password); this.set({ createError: error ?? '' }); if (error) return;
    const { username, password, role } = this.state;
    if (await this.perform('create', signal => api.post('/admin/team/users', { username, password, role }, { signal }), '账号已创建', 'createError')) { this.set({ username: '', password: '', role: 'Admin' }); await this.load(); }
  }
  private open(mode: Mode, user: User) { this.set({ mode, target: user, memberIds: [...user.groupIds], projectToAdd: '', modalPassword: '', modalRole: user.role, modalError: '' }); }
  private closeModal(event?: Event) { event?.preventDefault(); if (!this.state.busy) this.set({ mode: null, target: null, modalPassword: '', modalError: '' }); }
  private backdrop(event: MouseEvent) { const dialog = event.currentTarget as HTMLDialogElement; if (event.target !== dialog) return; const rect = dialog.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) this.closeModal(); }
  private async toggleUser(user: User) {
    if (!(await confirmDialog({ title: user.isActive ? '停用账号' : '启用账号', message: user.isActive ? `停用 ${user.username} 后，其全部会话立即失效。` : `重新启用 ${user.username}，该账号需要重新登录。`, confirmText: user.isActive ? '停用' : '启用', danger: user.isActive }))) return;
    if (await this.perform('user:'+user.id, signal => api.patch('/admin/team/users/'+user.id, { isActive: !user.isActive }, { signal }), '账号状态已更新')) await this.load();
  }
  private async saveModal(event: SubmitEvent) {
    event.preventDefault(); const error = validateForm(event.currentTarget as HTMLFormElement) ?? (this.state.mode === 'password' ? this.passwordValidation(this.state.modalPassword) : null); this.set({ modalError: error ?? '' }); if (error || !this.state.target) return;
    const { target, mode, memberIds, modalPassword, modalRole } = this.state;
    const work = (signal: AbortSignal) => mode === 'projects' ? api.put('/admin/team/users/'+target.id+'/projects', { groupIds: memberIds }, { signal }) : mode === 'password' ? api.post('/admin/team/users/'+target.id+'/password', { password: modalPassword }, { signal }) : api.patch('/admin/team/users/'+target.id, { role: modalRole }, { signal });
    if (mode !== 'projects' && !(await confirmDialog({ title: mode === 'password' ? '重置密码' : '修改角色', message: `修改 ${target.username} 后，该账号的全部会话立即失效。`, confirmText: '确认修改', danger: true }))) return;
    if (await this.perform('edit:'+target.id, work, '账号设置已保存', 'modalError')) { this.closeModal(); await this.load(); }
  }
  private async changePassword(event: SubmitEvent) {
    event.preventDefault(); let error = validateForm(event.currentTarget as HTMLFormElement);
    error ??= this.passwordValidation(this.state.newPassword);
    if (!error && this.state.newPassword !== this.state.confirmPassword) error = '两次输入的新密码不一致。';
    this.set({ passwordError: error ?? '' }); if (error || this.state.busy) return;
    const signal = this.requests.next('action'); this.set({ busy: 'password' });
    try { await api.post('/auth/password', { currentPassword: this.state.currentPassword, newPassword: this.state.newPassword }, { signal }); if (!signal.aborted && this.isConnected) { clearToken(); navigate('/login?redirect=%2Fteam', { replace: true }); } }
    catch (error) { if (!isAbort(error) && !signal.aborted && this.isConnected) this.set({ passwordError: error instanceof Error ? error.message : String(error) }); }
    finally { if (this.isConnected) this.set({ busy: '' }); this.requests.release('action', signal); }
  }
  private async revokeSession(session: Session) {
    if (!(await confirmDialog({ title: session.current ? '退出当前设备' : '退出设备', message: '退出后，此设备上的会话将立即失效。', confirmText: '退出', danger: true }))) return;
    if (await this.perform('session:'+session.id, signal => api.post('/auth/sessions/'+session.id+'/revoke', {}, { signal }), '设备已退出')) { if (session.current) { clearToken(); navigate('/login', { replace: true }); } else await this.load(); }
  }
  private async revokeOthers() {
    if (!(await confirmDialog({ title: '退出其他设备', message: '保留当前会话，立即退出此账号的其他登录设备。', confirmText: '退出其他设备', danger: true }))) return;
    if (await this.perform('others', signal => api.post('/auth/sessions/revoke-others', {}, { signal }), '其他设备已退出')) await this.load();
  }

  private modalContent() {
    const s = this.state;
    if (!s.target) return nothing;
    return html`<form class="stack" novalidate @submit=${this.saveModal}>
      ${s.mode === 'projects' ? html`<p class="muted">普通管理员可以管理以下项目的订单、令牌和回调。</p><div class="member-list">${s.memberIds.length ? s.memberIds.map(id => html`<span class="member-chip">${s.projects.find(project => project.id === id)?.name ?? `项目 #${id}`}<button type="button" aria-label=${`移除项目 ${id}`} ?disabled=${!!s.busy} @click=${() => this.set({ memberIds: s.memberIds.filter(value => value !== id) })}>×</button></span>`) : html`<p class="muted">尚未加入项目</p>`}</div>
        <div class="member-add"><div class="field"><label for="memberProject">添加项目</label><sp-select id="memberProject" label="添加项目" .value=${s.projectToAdd} .options=${[{ value: '', label: '选择项目' }, ...s.projects.filter(project => !s.memberIds.includes(project.id)).map(project => ({ value: String(project.id), label: project.name }))]} @change=${(event: Event) => this.set({ projectToAdd: (event.currentTarget as SpSelect).value })}></sp-select></div><button type="button" ?disabled=${!s.projectToAdd || !!s.busy} @click=${() => this.set({ memberIds: [...s.memberIds, Number(s.projectToAdd)], projectToAdd: '' })}>添加</button></div>
        ${s.target.role === 'SuperAdmin' ? html`<p class="action-note">超级管理员始终拥有全部项目权限。</p>` : nothing}` : s.mode === 'password' ? html`<div class="field"><label for="resetPassword">新密码</label><input id="resetPassword" type="password" required minlength="8" maxlength="72" autocomplete="new-password" .value=${s.modalPassword} @input=${(event: Event) => this.set({ modalPassword: (event.currentTarget as HTMLInputElement).value, modalError: '' })}></div>` : html`<div class="field"><label for="editRole">角色</label><sp-select id="editRole" label="角色" required .value=${s.modalRole} .options=${[{ value: 'Admin', label: '项目管理员' }, { value: 'SuperAdmin', label: '超级管理员' }]} @change=${(event: Event) => this.set({ modalRole: (event.currentTarget as SpSelect).value })}></sp-select></div>`}
      ${s.modalError ? html`<p class="err-text" role="alert">${s.modalError}</p>` : nothing}<div class="acts"><button type="button" ?disabled=${!!s.busy} @click=${this.closeModal}>取消</button><button class="primary" type="submit" ?disabled=${!!s.busy}>${s.busy ? '保存中…' : '保存'}</button></div>
    </form>`;
  }

  protected override render() {
    const s = this.state;
    if (!s.authenticated) return nothing;
    return html`<main class="wrap">
      <div class="team-head"><div class="page-head"><h1>团队与账号</h1><p>管理项目成员、账号权限和登录设备。</p></div><button type="button" ?disabled=${s.loading || !!s.busy} @click=${() => void this.load()}>刷新</button></div>
      ${s.error ? html`<p class="err-text" role="alert">${s.error}</p>` : nothing}
      <section class="panel"><h2>当前账号</h2><div class="account-meta"><strong>${s.account?.username ?? '加载中…'}</strong><span class="badge info">${this.superAdmin ? '超级管理员' : '项目管理员'}</span></div><p class="action-note">${this.superAdmin ? '可以管理全部项目及团队设置。' : '仅可管理已加入的项目；请联系超级管理员配置项目成员。'}</p></section>
      ${this.superAdmin ? html`<section class="panel"><h2>创建账号</h2><form class="team-form" novalidate @submit=${this.createUser}>
        <div class="field"><label for="newUsername">用户名</label><input id="newUsername" required minlength="3" maxlength="20" autocomplete="off" .value=${s.username} @input=${(event: Event) => this.set({ username: (event.currentTarget as HTMLInputElement).value, createError: '' })}></div>
        <div class="field"><label for="newAccountPassword">初始密码</label><input id="newAccountPassword" type="password" required minlength="8" maxlength="72" autocomplete="new-password" .value=${s.password} @input=${(event: Event) => this.set({ password: (event.currentTarget as HTMLInputElement).value, createError: '' })}></div>
        <div class="field"><label for="newRole">角色</label><sp-select id="newRole" label="角色" required .value=${s.role} .options=${[{ value: 'Admin', label: '项目管理员' }, { value: 'SuperAdmin', label: '超级管理员' }]} @change=${(event: Event) => this.set({ role: (event.currentTarget as SpSelect).value })}></sp-select></div><button class="primary" type="submit" ?disabled=${s.loading || !!s.busy}>${s.busy === 'create' ? '创建中…' : '创建账号'}</button></form>
        ${s.createError ? html`<p class="err-text" role="alert">${s.createError}</p>` : nothing}<p class="action-note">项目管理员创建后需要配置项目成员，才可管理相应项目。</p></section>
        <section class="panel"><h2>团队账号</h2><div class="table-scroll"><table class="team-table"><thead><tr><th>用户名</th><th>角色</th><th>项目</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead><tbody>${repeat(s.users, user => user.id, user => html`<tr data-user-id=${user.id}><td>${user.username}${user.id === s.account?.userId ? html`<span class="small muted">（当前）</span>` : nothing}</td><td>${user.role === 'SuperAdmin' ? '超级管理员' : '项目管理员'}</td><td class="project-names">${user.role === 'SuperAdmin' ? '全部项目' : user.groupIds.length ? user.groupIds.map(id => html`<span class="project-name">${s.projects.find(project => project.id === id)?.name ?? `项目 #${id}`}</span>`) : '未加入项目'}</td><td><span class=${`badge ${user.isActive ? 'ok' : 'err'}`}>${user.isActive ? '启用' : '停用'}</span></td><td>${dateTime(user.createdAt)}</td><td><div class="user-actions"><button type="button" ?disabled=${!!s.busy} @click=${() => this.open('projects', user)}>项目成员</button><button type="button" ?disabled=${!!s.busy} @click=${() => this.open('role', user)}>修改角色</button><button type="button" ?disabled=${!!s.busy} @click=${() => this.open('password', user)}>重置密码</button><button type="button" class=${user.isActive ? 'danger' : ''} ?disabled=${!!s.busy || user.id === s.account?.userId} @click=${() => void this.toggleUser(user)}>${user.isActive ? '停用' : '启用'}</button></div></td></tr>`)}</tbody></table></div></section>` : nothing}
      <div class="team-columns"><section class="panel"><h2>修改我的密码</h2><form class="stack password-form" novalidate @submit=${this.changePassword}>
        <div class="field"><label for="currentPassword">当前密码</label><input id="currentPassword" type="password" required maxlength="72" autocomplete="current-password" .value=${s.currentPassword} @input=${(event: Event) => this.set({ currentPassword: (event.currentTarget as HTMLInputElement).value, passwordError: '' })}></div>
        <div class="field"><label for="newPassword">新密码</label><input id="newPassword" type="password" required minlength="8" maxlength="72" autocomplete="new-password" .value=${s.newPassword} @input=${(event: Event) => this.set({ newPassword: (event.currentTarget as HTMLInputElement).value, passwordError: '' })}></div>
        <div class="field"><label for="confirmPassword">确认新密码</label><input id="confirmPassword" type="password" required minlength="8" maxlength="72" autocomplete="new-password" .value=${s.confirmPassword} @input=${(event: Event) => this.set({ confirmPassword: (event.currentTarget as HTMLInputElement).value, passwordError: '' })}></div>
        ${s.passwordError ? html`<p class="err-text" role="alert">${s.passwordError}</p>` : nothing}<p class="action-note">保存后会退出此账号的所有设备，需要重新登录。</p><div class="acts"><button type="submit" class="primary" ?disabled=${s.loading || !!s.busy}>修改密码并重新登录</button></div></form></section>
        <section class="panel"><div class="panel-head"><h2>登录设备</h2><button type="button" ?disabled=${s.loading || !!s.busy || s.sessions.length < 2} @click=${() => void this.revokeOthers()}>退出其他设备</button></div><div class="table-scroll"><table class="session-table"><thead><tr><th>设备与来源</th><th>最近访问</th><th>操作</th></tr></thead><tbody>${repeat(s.sessions, session => session.id, session => html`<tr data-session-id=${session.id}><td><div class="session-device">${session.userAgent || '未知设备'}</div><div class="session-meta"><span class="mono small">${session.clientIp || '未记录 IP'}</span>${session.current ? html`<span class="badge info session-current">当前设备</span>` : nothing}</div><div class="small muted session-created">登录于 ${dateTime(session.createdAt)}</div></td><td>${dateTime(session.lastSeenAt)}</td><td><button type="button" ?disabled=${!!s.busy} @click=${() => void this.revokeSession(session)}>退出</button></td></tr>`)}</tbody></table></div>${!s.sessions.length ? html`<p class="muted">${s.loading ? '加载中…' : '没有有效会话'}</p>` : nothing}</section></div>
    </main><dialog class="modal team-modal" aria-labelledby="team-modal-title" @click=${this.backdrop} @cancel=${this.closeModal} @close=${(event: Event) => { if (!(event.currentTarget as HTMLDialogElement).open) this.closeModal(event); }}><div class="modal-header"><h3 id="team-modal-title">${s.mode === 'projects' ? '配置项目成员' : s.mode === 'password' ? '重置密码' : '修改角色'} · ${s.target?.username ?? ''}</h3><button data-close type="button" class="modal-close" aria-label="关闭弹窗" ?disabled=${!!s.busy} @click=${this.closeModal}>×</button></div>${this.modalContent()}</dialog>`;
  }
}
customElements.define('sp-team-page', SpTeamPage);
