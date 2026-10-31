import { LitElement, html, nothing, type PropertyValues } from 'lit';
import type { SpSelect } from './app';
import { repeat } from 'lit/directives/repeat.js';
import { api, confirmDialog, dateTime, fail, isAbort, ok, requireAuth } from './app';
import type { APIKeyDTO, GroupWithKeys, IssuedAPIKey } from './api-types';
import { RequestController } from './controllers/requests';
import { modalStyles, pageStyles, sharedStyles } from './styles/shared';
import { tokenStyles } from './styles/tokens';
import { validateForm } from './services/form';

type ModalMode = 'create' | 'receipt' | 'webhook' | null;

class SpGroupsPage extends LitElement {
  static override styles = [sharedStyles, modalStyles, pageStyles, tokenStyles];
  static override properties = {
    authenticated: { state: true }, superAdmin: { state: true }, groups: { state: true }, tokens: { state: true },
    pending: { state: true }, loading: { state: true }, listError: { state: true },
    projectFilter: { state: true }, projectName: { state: true }, projectError: { state: true },
    modalMode: { state: true }, tokenName: { state: true }, tokenProject: { state: true },
    issuedToken: { state: true }, modalError: { state: true },
    webhookGroup: { state: true }, webhookSecret: { state: true }, webhookLoading: { state: true },
    copied: { state: true },
  };

  declare authenticated: boolean;
  declare superAdmin: boolean;
  declare groups: GroupWithKeys[];
  declare tokens: APIKeyDTO[];
  declare pending: ReadonlySet<string>;
  declare loading: boolean;
  declare listError: string;
  declare projectFilter: string;
  declare projectName: string;
  declare projectError: string;
  declare modalMode: ModalMode;
  declare tokenName: string;
  declare tokenProject: string;
  declare issuedToken: IssuedAPIKey | null;
  declare modalError: string;
  declare webhookGroup: GroupWithKeys | null;
  declare webhookSecret: string;
  declare webhookLoading: boolean;
  declare copied: boolean;
  private webhookRequest: AbortSignal | null = null;
  private readonly requests = new RequestController(this, () => {
    this.clearSecrets();
    if (!this.pending.size) void this.load();
  });

  constructor() {
    super();
    this.authenticated = false;
    this.superAdmin = false;
    this.groups = [];
    this.tokens = [];
    this.pending = new Set();
    this.loading = false;
    this.listError = '';
    this.projectFilter = '';
    this.projectName = '';
    this.projectError = '';
    this.modalMode = null;
    this.tokenName = '';
    this.tokenProject = '';
    this.issuedToken = null;
    this.modalError = '';
    this.webhookGroup = null;
    this.webhookSecret = '';
    this.webhookLoading = false;
    this.copied = false;
  }

  override connectedCallback() {
    super.connectedCallback();
    this.authenticated = requireAuth();
    if (this.authenticated) void this.load();
  }

  override disconnectedCallback() {
    this.clearSecrets();
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    this.pending = new Set();
    super.disconnectedCallback();
  }

  protected override updated(changed: PropertyValues<this>) {
    if (!this.isConnected || !changed.has('modalMode')) return;
    const dialog = this.renderRoot.querySelector<HTMLDialogElement>('dialog');
    if (!dialog) return;
    if (this.modalMode) {
      if (!dialog.open) dialog.showModal();
      dialog.querySelector<HTMLElement>(this.modalMode === 'create' ? '#tokenName' : '[data-copy-secret], [data-dismiss]')?.focus();
    } else if (dialog.open) dialog.close();
  }

  private get defaultProject() {
    return this.groups.reduce<GroupWithKeys | null>((current, group) => !current || group.id < current.id ? group : current, null);
  }

  private clearSecrets() {
    for (const field of this.renderRoot.querySelectorAll('[data-issued-secret], [data-webhook-secret]')) {
      for (const node of field.childNodes) if (node.nodeType === Node.TEXT_NODE) node.textContent = '';
    }
    this.modalMode = null;
    this.issuedToken = null;
    this.webhookGroup = null;
    this.webhookSecret = '';
    this.webhookRequest = null;
    this.webhookLoading = false;
    this.modalError = '';
    this.copied = false;
    this.requests.abort('webhook');
  }

  private closeModal() {
    if (this.pending.has('create-token')) return;
    this.clearSecrets();
  }

  private backdrop(event: MouseEvent) {
    const dialog = event.currentTarget as HTMLDialogElement;
    if (event.target !== dialog) return;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) this.closeModal();
  }

  private openCreate() {
    if (this.loading || this.pending.has('create-token') || !this.groups.some((group) => group.isActive)) return;
    this.clearSecrets();
    this.tokenName = '';
    this.tokenProject = '';
    this.modalMode = 'create';
  }

  private async copySecret() {
    const value = this.modalMode === 'receipt' ? this.issuedToken?.key : this.webhookSecret;
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      if (this.isConnected && (this.issuedToken?.key === value || this.webhookSecret === value)) this.copied = true;
    } catch {
      if (this.isConnected) fail('复制失败，可选中密钥手动复制');
    }
  }

  private async load() {
    if (!this.authenticated || !this.isConnected || this.pending.size) return;
    const signal = this.requests.next('list');
    this.loading = true;
    this.listError = '';
    try {
      const [groups, tokens, account] = await Promise.all([
        api.get<GroupWithKeys[]>('/admin/groups', { signal }),
        api.get<APIKeyDTO[]>('/admin/api-keys', { signal }),
        api.get<{ role: string }>('/auth/session', { signal }),
      ]);
      if (signal.aborted || !this.isConnected) return;
      this.groups = groups;
      this.superAdmin = account.role === 'SuperAdmin';
      this.tokens = tokens.map((token) => this.tokenMetadata(token)).sort((left, right) => right.id - left.id);
      if (this.projectFilter && !groups.some((group) => String(group.id) === this.projectFilter)) this.projectFilter = '';
    } catch (error) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) this.listError = error instanceof Error ? error.message : String(error);
    } finally {
      if (!signal.aborted) this.loading = false;
      this.requests.release('list', signal);
    }
  }

  private tokenMetadata(token: APIKeyDTO): APIKeyDTO {
    const { key: _secret, ...metadata } = token;
    const project = this.groups.find((group) => group.id === token.groupId);
    return { ...metadata, groupName: project?.name ?? metadata.groupName, groupIsActive: project?.isActive ?? metadata.groupIsActive };
  }

  private async runAction(id: string, action: (signal: AbortSignal) => Promise<void>) {
    if (this.pending.has(id)) return;
    this.requests.abort('list');
    this.loading = false;
    const signal = this.requests.next(id);
    this.pending = new Set([...this.pending, id]);
    try {
      await action(signal);
    } catch (error) {
      if (!isAbort(error) && this.isConnected) {
        if (id === 'create-token') this.modalError = error instanceof Error ? error.message : String(error);
        else fail(error);
      }
    } finally {
      this.requests.release(id, signal);
      const pending = new Set(this.pending);
      pending.delete(id);
      this.pending = pending;
    }
  }

  private createToken(event: SubmitEvent) {
    event.preventDefault();
    if (this.loading || this.pending.has('create-token')) return;
    this.modalError = validateForm(event.currentTarget as HTMLFormElement) || '';
    if (this.modalError) return;
    const selectedProject = this.tokenProject ? this.groups.find((group) => String(group.id) === this.tokenProject) : this.defaultProject;
    if (!selectedProject) {
      this.modalError = '暂无可访问项目，请联系超级管理员分配项目。';
      return;
    }
    if (!selectedProject.isActive) {
      this.modalError = '当前项目已停用，请选择其他启用项目，或在项目设置中启用它。';
      return;
    }
    const name = this.tokenName.trim();
    void this.runAction('create-token', async (signal) => {
      const issued = await api.post<IssuedAPIKey>('/admin/api-keys', {
        ...(name ? { name } : {}), groupId: selectedProject.id,
      }, { signal });
      if (signal.aborted || !this.isConnected) return;
      if (!issued.key) throw new Error('创建成功但未收到完整令牌，请刷新列表后重新创建');
      const key = issued.key;
      const metadata = this.tokenMetadata(issued);
      this.tokens = [...this.tokens.filter((token) => token.id !== metadata.id), metadata].sort((left, right) => right.id - left.id);
      this.issuedToken = { ...metadata, key };
      this.modalMode = 'receipt';
      this.copied = false;
    });
  }

  private async toggleToken(token: APIKeyDTO) {
    await this.runAction(`token:${token.id}`, async (signal) => {
      if (token.isActive && !await confirmDialog({ title: '停用令牌',
        message: `停用「${token.name}」后，该令牌将无法调用 API。已有订单和项目回调继续处理。`,
        confirmText: '确认停用', danger: true })) return;
      if (signal.aborted || !this.isConnected) return;
      const updated = await api.post<APIKeyDTO>(`/admin/api-keys/${token.id}/toggle`, {}, { signal });
      if (signal.aborted || !this.isConnected) return;
      const metadata = this.tokenMetadata(updated);
      this.tokens = this.tokens.map((item) => item.id === metadata.id ? metadata : item);
      ok(updated.isActive ? '令牌已启用' : '令牌已停用');
    });
  }

  private async toggleProject(group: GroupWithKeys) {
    await this.runAction(`project:${group.id}`, async (signal) => {
      if (group.isActive && !await confirmDialog({ title: '停用项目',
        message: `停用「${group.name}」会禁止该项目下所有令牌调用 API。已有订单和回调继续处理。`,
        confirmText: '确认停用', danger: true })) return;
      if (signal.aborted || !this.isConnected) return;
      const updated = await api.post<GroupWithKeys>(`/admin/groups/${group.id}/toggle`, {}, { signal });
      if (signal.aborted || !this.isConnected) return;
      this.groups = this.groups.map((item) => item.id === updated.id ? { ...item, isActive: updated.isActive } : item);
      this.tokens = this.tokens.map((token) => token.groupId === updated.id ? { ...token, groupIsActive: updated.isActive } : token);
      ok(updated.isActive ? '项目已启用' : '项目已停用');
    });
  }

  private createProject(event: SubmitEvent) {
    event.preventDefault();
    if (!this.superAdmin || this.loading || this.pending.has('create-project')) return;
    this.projectError = validateForm(event.currentTarget as HTMLFormElement) || '';
    if (this.projectError) return;
    const name = this.projectName.trim();
    if (!name) { this.projectError = '请填写项目名称。'; return; }
    void this.runAction('create-project', async (signal) => {
      const group = await api.post<GroupWithKeys>('/admin/projects', { name }, { signal });
      if (signal.aborted || !this.isConnected) return;
      this.groups = [...this.groups.filter((item) => item.id !== group.id), group].sort((left, right) => left.id - right.id);
      this.projectName = '';
      ok('项目已创建，可在创建令牌时选择');
    });
  }

  private async openWebhook(group: GroupWithKeys) {
    this.clearSecrets();
    this.modalMode = 'webhook';
    this.webhookGroup = group;
    this.webhookLoading = true;
    const signal = this.requests.next('webhook');
    this.webhookRequest = signal;
    try {
      const result = await api.get<{ secret: string }>(`/admin/groups/${group.id}/webhook-secret`, { signal });
      if (signal.aborted || this.webhookRequest !== signal || !this.isConnected) return;
      if (!result.secret) throw new Error('未收到项目回调密钥');
      this.webhookSecret = result.secret;
    } catch (error) {
      if (!isAbort(error) && this.webhookRequest === signal && this.isConnected) this.modalError = error instanceof Error ? error.message : String(error);
    } finally {
      if (this.webhookRequest === signal) this.webhookLoading = false;
      this.requests.release('webhook', signal);
    }
  }

  private modalContent() {
    const busy = this.pending.has('create-token');
    const selectedProject = this.tokenProject ? this.groups.find((group) => String(group.id) === this.tokenProject) : this.defaultProject;
    if (this.modalMode === 'create') return html`
      <form class="token-form" novalidate @submit=${this.createToken}>
        <div class="field"><label for="tokenName">备注（选填）</label>
          <input id="tokenName" maxlength="100" placeholder="例如：本地开发 / CI" .value=${this.tokenName}
            ?disabled=${busy} @input=${(event: Event) => { this.tokenName = (event.currentTarget as HTMLInputElement).value; }}></div>
        <p class="token-note">${selectedProject && !selectedProject.isActive
          ? '当前项目已停用，请选择其他启用项目，或在项目设置中启用它。'
          : `归属 ${selectedProject?.name || '默认项目'}，创建后即可调用 API。`}</p>
        <details class="advanced"><summary>选择其他项目（可选）</summary>
          <div class="field"><label for="tokenProject">项目</label>
            <sp-select id="tokenProject" label="项目" .value=${this.tokenProject} ?disabled=${busy}
              .options=${[{ value: '', label: `${this.superAdmin ? '默认项目' : '默认选择'}${this.defaultProject ? ` · ${this.defaultProject.name}` : ''}` },
                ...this.groups.filter((group) => group.isActive).map((group) => ({ value: String(group.id), label: group.name }))]}
              @change=${(event: Event) => { this.tokenProject = (event.currentTarget as SpSelect).value; }}></sp-select></div></details>
        ${this.modalError ? html`<p class="err-text form-error" role="alert">${this.modalError}</p>` : nothing}
        <div class="acts"><button type="button" ?disabled=${busy} @click=${this.closeModal}>取消</button>
          <button class="primary" type="submit" ?disabled=${busy || !selectedProject?.isActive}>${busy ? '创建中…' : '创建令牌'}</button></div>
      </form>`;
    if (this.modalMode === 'receipt' && this.issuedToken) return html`
      <div class="secret-receipt">
        <p class="receipt-note"><strong>请立即复制并保存。</strong>完整令牌只在这次创建后显示，关闭后无法再次查看。</p>
        <pre class="secret-box mono" data-issued-secret>${this.issuedToken.key}</pre>
        <p class="token-note">${this.issuedToken.groupName} / ${this.issuedToken.name}</p>
        <div class="connection-example"><p class="token-note">调用 API 时，在请求头中传入：</p><code>Authorization: Bearer &lt;令牌&gt;</code></div>
        <div class="acts"><button type="button" data-dismiss @click=${this.closeModal}>完成</button>
          <button class="primary" type="button" data-copy-secret @click=${this.copySecret}>${this.copied ? '已复制' : '复制令牌'}</button></div>
      </div>`;
    if (this.modalMode === 'webhook') return html`
      <div class="secret-receipt">
        <p class="receipt-note">${this.webhookGroup?.name} 的回调签名密钥。接收支付或退款回调时，用它验证签名；API 调用继续使用令牌。</p>
        ${this.webhookLoading ? html`<p role="status" class="token-note">正在读取回调密钥…</p>` : nothing}
        ${this.webhookSecret ? html`<pre class="secret-box mono" data-webhook-secret>${this.webhookSecret}</pre>
          <p class="token-note">令牌轮换和停用不会改变这个密钥。升级前的订单继续使用原有签名规则。</p>` : nothing}
        ${this.modalError ? html`<p class="err-text form-error" role="alert">${this.modalError}</p>
          <button type="button" @click=${() => this.webhookGroup && void this.openWebhook(this.webhookGroup)}>重试</button>` : nothing}
        <div class="acts"><button type="button" data-dismiss @click=${this.closeModal}>关闭</button>
          ${this.webhookSecret ? html`<button class="primary" type="button" data-copy-secret @click=${this.copySecret}>${this.copied ? '已复制' : '复制回调密钥'}</button>` : nothing}</div>
      </div>`;
    return nothing;
  }

  protected override render() {
    if (!this.authenticated) return nothing;
    const tokens = this.projectFilter ? this.tokens.filter((token) => String(token.groupId) === this.projectFilter) : this.tokens;
    const canCreateToken = this.groups.some((group) => group.isActive);
    const title = this.modalMode === 'create' ? '创建令牌' : this.modalMode === 'receipt' ? '令牌已创建' : '项目回调密钥';
    return html`
      <main class="wrap">
        <div class="tokens-head"><div class="page-head"><h1>API令牌</h1><p>创建令牌即可接入，项目回调使用独立签名密钥。</p></div>
          <div class="head-actions"><button type="button" data-refresh ?disabled=${this.loading || this.pending.size > 0} @click=${() => void this.load()}>${this.loading ? '加载中…' : '刷新'}</button>
            <button type="button" class="primary" data-create-token ?disabled=${this.loading || this.pending.has('create-token') || !canCreateToken} @click=${this.openCreate}>创建令牌</button></div></div>
        ${this.listError ? html`<section class="panel list-error" role="alert"><p>令牌列表加载失败：${this.listError}</p>
          <button type="button" ?disabled=${this.loading || this.pending.size > 0} @click=${() => void this.load()}>重试</button></section>` : nothing}
        <section class="panel tokens-panel">
          <div class="panel-heading"><h2>令牌列表 <span class="default-label">${tokens.length} 个</span></h2>
            ${this.groups.length > 1 ? html`<label class="project-filter">项目<sp-select label="筛选项目" data-project-filter .value=${this.projectFilter}
              .options=${[{ value: '', label: '全部项目' }, ...this.groups.map((group) => ({ value: String(group.id), label: group.name }))]}
              @change=${(event: Event) => { this.projectFilter = (event.currentTarget as SpSelect).value; }}></sp-select></label>` : nothing}</div>
          ${!this.loading && !this.listError && !canCreateToken ? html`<p class="token-note" role="status">${this.groups.length ? '当前可访问项目均已停用，启用项目后可创建令牌。' : this.superAdmin ? '暂无项目，请先创建项目。' : '暂无可访问项目，请联系超级管理员分配项目。'}</p>` : nothing}
          ${tokens.length ? html`<div class="table-scroll"><table>
            <thead><tr><th>备注</th><th>令牌</th><th>项目</th><th>状态</th><th>最近使用</th><th>创建时间</th><th></th></tr></thead>
            <tbody>${repeat(tokens, (token) => token.id, (token) => html`<tr data-token-id=${token.id}>
              <td class="token-name">${token.name}</td><td class="mono token-prefix">${token.keyPrefix}…${token.keySuffix}</td><td>${token.groupName}</td>
              <td><span data-token-status class=${`badge ${token.isActive && token.groupIsActive ? 'ok' : 'err'}`}>${!token.isActive ? '已停用' : token.groupIsActive ? '可用' : '项目已停用'}</span></td>
              <td class="token-time">${token.lastUsedAt ? dateTime(token.lastUsedAt) : '从未'}</td><td class="token-time">${dateTime(token.createdAt)}</td>
              <td class="acts"><a data-token-usage href=${`/notifications?apiKeyId=${token.id}`} aria-label=${`查看 ${token.name} 的使用日志`}>使用日志</a>
                <button type="button" data-toggle-token class=${token.isActive ? 'danger' : ''}
                ?disabled=${this.pending.has(`token:${token.id}`)} @click=${() => void this.toggleToken(token)}>${token.isActive ? '停用' : '启用'}</button></td>
            </tr>`)}</tbody></table></div>` : html`<div class="empty-tokens"><p>${this.loading ? '正在加载令牌…' : this.listError ? '列表暂不可用，请重试。' : '暂无令牌，创建一枚即可开始调用 API。'}</p>
              ${!this.loading && !this.listError && canCreateToken ? html`<button type="button" class="primary" @click=${this.openCreate}>创建令牌</button>` : nothing}</div>`}
          <p class="token-note">完整令牌仅创建时显示。停用令牌会禁止新 API 请求，已有订单和回调继续处理。</p>
        </section>
        <details class="panel projects"><summary>项目与回调设置</summary><div class="projects-body">
          <p class="token-note">${this.superAdmin ? '默认项目即可使用。需要隔离不同应用的数据时，再创建项目。' : '当前显示已分配给你的项目。如需新增项目或调整权限，请联系超级管理员。'}</p>
          <div class="project-list">${repeat(this.groups, (group) => group.id, (group) => html`<section class="project-item" data-project-id=${group.id}>
            <div class="project-heading"><div class="project-title"><strong>${group.name}</strong>${group.id === this.defaultProject?.id ? html`<span class="default-label">${this.superAdmin ? '默认项目' : '默认选择'}</span>` : nothing}
              <span class=${`badge ${group.isActive ? 'ok' : 'err'}`}>${group.isActive ? '启用' : '已停用'}</span></div>
              <div class="project-actions"><button type="button" data-webhook @click=${() => void this.openWebhook(group)}>查看回调密钥</button>
                <button type="button" data-toggle-project class=${group.isActive ? 'danger' : ''} ?disabled=${this.pending.has(`project:${group.id}`)}
                  @click=${() => void this.toggleProject(group)}>${group.isActive ? '停用项目' : '启用项目'}</button></div></div>
            ${!group.isActive ? html`<p class="token-note">该项目的全部令牌暂不可调用 API。启用项目后，未被单独停用的令牌恢复可用。</p>` : nothing}
          </section>`)}</div>
          ${this.superAdmin ? html`<form class="row project-form" novalidate @submit=${this.createProject}><div class="field grow"><label for="projectName">项目名称</label>
            <input id="projectName" required maxlength="100" placeholder="例如：另一个应用" .value=${this.projectName} ?disabled=${this.pending.has('create-project')}
              aria-invalid=${this.projectError ? 'true' : nothing} aria-describedby=${this.projectError ? 'project-error' : nothing}
              @input=${(event: Event) => { this.projectName = (event.currentTarget as HTMLInputElement).value; this.projectError = ''; }}></div>
            <button type="submit" ?disabled=${this.loading || this.pending.has('create-project')}>${this.pending.has('create-project') ? '创建中…' : '创建项目'}</button></form>
          ${this.projectError ? html`<p class="err-text small" id="project-error" role="alert">${this.projectError}</p>` : nothing}` : nothing}
        </div></details>
      </main>
      <dialog class="modal token-modal" aria-labelledby="token-modal-title" @click=${this.backdrop}
        @close=${this.closeModal} @cancel=${(event: Event) => { event.preventDefault(); this.closeModal(); }}>
        <div class="modal-header"><h3 id="token-modal-title">${title}</h3><button type="button" class="modal-close" aria-label="关闭弹窗"
          ?disabled=${this.pending.has('create-token')} @click=${this.closeModal}>×</button></div>
        ${this.modalContent()}
      </dialog>`;
  }
}

customElements.define('sp-groups-page', SpGroupsPage);
