import { LitElement, html, nothing, type PropertyValues, type TemplateResult } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { api, ok, fail, isAbort, requireAuth, dateTime, count, qs } from './app';
import type { SpSelect, SpDatePicker } from './app';
import type { APIKeyDTO, GroupWithKeys, AdminNotification, AdminNotifyPage, AdminUsageLog, AdminUsagePage, AdminUsageSummary } from './api-types';
import { sharedStyles, modalStyles, pageStyles } from './styles/shared';
import { usageStyles } from './styles/usage';
import { RequestController } from './controllers/requests';
import { cleanLocationValue, locationSummary, networkLabel } from './services/location';
import { validateForm } from './services/form';

const PAGE_SIZE = 50;
const STATUS: Readonly<Record<number, readonly [string, string]>> = {
  0: ['待发送', 'info'], 1: ['成功', 'ok'], 2: ['失败重试中', 'warn'], 3: ['重试耗尽', 'err'],
};
type Tab = 'api' | 'callbacks';
type UsageFilters = { groupId: string; apiKeyId: string; method: string; status: string; keyword: string; dateFrom: string; dateTo: string };
type Project = Pick<GroupWithKeys, 'id' | 'name'>;
type Token = Pick<APIKeyDTO, 'id' | 'name' | 'groupId' | 'keyPrefix' | 'keySuffix'>;
type CallbackDetail = AdminNotification & {
  body: string; simulationEnabled: boolean; nextRetryAt?: string; availableAt?: string;
  sourceNotificationId?: number; simulationMode?: 'duplicate' | 'delayed';
  attempts: { id: number; attemptNo: number; httpStatus?: number; responseSummary: string; durationMs: number; errorMessage: string; createdAt: string }[];
};
type SimulationResult = { id: number; sourceNotificationId: number; mode: 'duplicate' | 'delayed'; availableAt: string; orderNo?: string };
const emptyFilters = (): UsageFilters => ({ groupId: '', apiKeyId: '', method: '', status: '', keyword: '', dateFrom: '', dateTo: '' });
const positiveId = (value: string | null) => value && /^[1-9]\d*$/.test(value) ? value : '';
const successful = (log: AdminUsageLog) => log.statusCode >= 200 && log.statusCode < 400;
const duration = (value: number) => new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 }).format(value) + ' ms';

class SpNotificationsPage extends LitElement {
  static override properties = {
    authorized: { state: true }, tab: { state: true },
    selectedStatus: { state: true }, appliedStatus: { state: true }, selectedOrderNo: { state: true }, appliedOrderNo: { state: true },
    items: { state: true }, page: { state: true }, pageSize: { state: true }, total: { state: true },
    loading: { state: true }, errorMessage: { state: true }, busy: { state: true },
    usageFilters: { state: true }, appliedUsageFilters: { state: true }, usageItems: { state: true },
    usagePage: { state: true }, usagePageSize: { state: true }, usageTotal: { state: true }, summary: { state: true },
    usageLoading: { state: true }, usageError: { state: true }, filterError: { state: true },
    projects: { state: true }, tokens: { state: true }, metadataError: { state: true },
    selectedLog: { state: true }, copiedValue: { state: true }, copyError: { state: true },
    selectedCallback: { state: true }, callbackDetail: { state: true }, callbackLoading: { state: true }, callbackError: { state: true },
    callbackDelay: { state: true }, callbackFormError: { state: true }, simulationResult: { state: true },
  };
  static override styles = [sharedStyles, modalStyles, pageStyles, usageStyles];

  declare private authorized: boolean;
  declare private tab: Tab;
  declare private selectedStatus: string;
  declare private appliedStatus: string;
  declare private selectedOrderNo: string;
  declare private appliedOrderNo: string;
  declare private items: AdminNotification[];
  declare private page: number;
  declare private pageSize: number;
  declare private total: number;
  declare private loading: boolean;
  declare private errorMessage: string;
  declare private busy: ReadonlySet<number>;
  declare private usageFilters: UsageFilters;
  declare private appliedUsageFilters: UsageFilters;
  declare private usageItems: AdminUsageLog[];
  declare private usagePage: number;
  declare private usagePageSize: number;
  declare private usageTotal: number;
  declare private summary: AdminUsageSummary | null;
  declare private usageLoading: boolean;
  declare private usageError: string;
  declare private filterError: string;
  declare private projects: Project[];
  declare private tokens: Token[];
  declare private metadataError: string;
  declare private selectedLog: AdminUsageLog | null;
  declare private copiedValue: string;
  declare private copyError: string;
  declare private selectedCallback: AdminNotification | null;
  declare private callbackDetail: CallbackDetail | null;
  declare private callbackLoading: boolean;
  declare private callbackError: string;
  declare private callbackDelay: string;
  declare private callbackFormError: string;
  declare private simulationResult: SimulationResult | null;
  private listRequest: AbortSignal | null = null;
  private readonly actions = new Map<number, AbortSignal>();
  private detailOpener: HTMLElement | null = null;
  private readonly requests = new RequestController(this, () => { this.closeDetail(); void this.refresh(); });

  constructor() {
    super();
    const params = new URLSearchParams(location.search);
    const status = params.get('status') ?? '';
    this.authorized = false;
    this.tab = params.get('tab') === 'callbacks' || (params.get('tab') !== 'api' && (params.has('orderNo') || ['0', '1', '2', '3'].includes(status))) ? 'callbacks' : 'api';
    this.selectedStatus = this.appliedStatus = ['0', '1', '2', '3'].includes(status) ? status : '';
    this.selectedOrderNo = this.appliedOrderNo = params.get('orderNo') ?? '';
    this.items = [];
    this.page = 1;
    this.pageSize = PAGE_SIZE;
    this.total = 0;
    this.loading = false;
    this.errorMessage = '';
    this.busy = new Set();
    this.usageFilters = {
      groupId: positiveId(params.get('groupId')), apiKeyId: positiveId(params.get('apiKeyId')),
      method: params.get('method')?.toUpperCase() ?? '', status: ['success', 'failure'].includes(status) ? status : '',
      keyword: params.get('keyword') ?? '', dateFrom: params.get('dateFrom') ?? '', dateTo: params.get('dateTo') ?? '',
    };
    this.appliedUsageFilters = { ...this.usageFilters };
    this.usageItems = [];
    this.usagePage = 1;
    this.usagePageSize = PAGE_SIZE;
    this.usageTotal = 0;
    this.summary = null;
    this.usageLoading = false;
    this.usageError = this.filterError = this.metadataError = '';
    this.projects = [];
    this.tokens = [];
    this.selectedLog = null;
    this.selectedCallback = this.callbackDetail = null;
    this.callbackLoading = false;
    this.callbackError = this.callbackFormError = '';
    this.callbackDelay = '30';
    this.simulationResult = null;
    this.copiedValue = this.copyError = '';
  }

  override connectedCallback() {
    super.connectedCallback();
    this.authorized = requireAuth();
    if (this.authorized) void this.refresh();
  }

  override disconnectedCallback() {
    this.listRequest = null;
    this.actions.clear();
    this.busy = new Set();
    this.loading = this.usageLoading = false;
    this.selectedLog = null;
    this.selectedCallback = this.callbackDetail = null;
    this.detailOpener = null;
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    super.disconnectedCallback();
  }

  protected override updated(changed: PropertyValues) {
    if (!this.isConnected || !changed.has('selectedLog') && !changed.has('selectedCallback')) return;
    const dialog = this.renderRoot.querySelector<HTMLDialogElement>('dialog');
    if (!dialog) return;
    if (this.selectedLog || this.selectedCallback) {
      if (!dialog.open) dialog.showModal();
      dialog.querySelector<HTMLElement>('[data-close-detail]')?.focus();
    } else if (dialog.open) {
      dialog.close();
      this.detailOpener?.focus();
      this.detailOpener = null;
    }
  }

  private async refresh() {
    await Promise.all([this.load(this.tab === 'api' ? this.usagePage : this.page), this.loadMetadata()]);
  }

  private async loadMetadata() {
    if (!this.isConnected || !this.authorized) return;
    const signal = this.requests.next('metadata');
    this.metadataError = '';
    try {
      const [groups, tokens] = await Promise.all([
        api.get<GroupWithKeys[]>('/admin/groups', { signal }), api.get<APIKeyDTO[]>('/admin/api-keys', { signal }),
      ]);
      if (signal.aborted || !this.isConnected) return;
      this.projects = groups.map(({ id, name }) => ({ id, name }));
      this.tokens = tokens.map(({ id, name, groupId, keyPrefix, keySuffix }) => ({ id, name, groupId, keyPrefix, keySuffix }));
    } catch (error: unknown) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) this.metadataError = '项目与令牌选项加载失败，可刷新重试。';
    } finally { this.requests.release('metadata', signal); }
  }

  private updateURL() {
    const url = new URL(location.href);
    for (const key of ['tab', 'orderNo', 'status', ...Object.keys(emptyFilters())]) url.searchParams.delete(key);
    url.searchParams.set('tab', this.tab);
    const values = this.tab === 'callbacks' ? { orderNo: this.appliedOrderNo, status: this.appliedStatus } : this.appliedUsageFilters;
    for (const [key, value] of Object.entries(values)) if (value) url.searchParams.set(key, value);
    history.replaceState(history.state, '', url);
  }

  private switchTab(tab: Tab) {
    if (this.tab === tab) return;
    this.closeDetail();
    this.requests.abort('list');
    this.listRequest = null;
    this.loading = this.usageLoading = false;
    this.tab = tab;
    this.updateURL();
    void this.load(tab === 'api' ? this.usagePage : this.page);
  }

  private tabKey(event: KeyboardEvent) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const tab: Tab = event.key === 'Home' ? 'api' : event.key === 'End' ? 'callbacks' : this.tab === 'api' ? 'callbacks' : 'api';
    this.switchTab(tab);
    this.renderRoot.querySelector<HTMLElement>(tab === 'api' ? '#api-tab' : '#callbacks-tab')?.focus();
  }

  private setFilter(key: keyof UsageFilters, value: string) {
    this.usageFilters = { ...this.usageFilters, [key]: value, ...(key === 'groupId' ? { apiKeyId: '' } : {}) };
    this.filterError = '';
  }

  private submitUsageFilters(event: SubmitEvent) {
    event.preventDefault();
    this.filterError = validateForm(event.currentTarget as HTMLFormElement) ?? '';
    if (this.filterError) return;
    if (this.usageFilters.dateFrom && this.usageFilters.dateTo && this.usageFilters.dateFrom > this.usageFilters.dateTo) {
      this.filterError = '开始日期不能晚于结束日期。';
      return;
    }
    this.filterError = '';
    this.appliedUsageFilters = { ...this.usageFilters, keyword: this.usageFilters.keyword.trim() };
    this.updateURL();
    void this.load(1);
  }

  private submitCallbackFilters(event: SubmitEvent) {
    event.preventDefault();
    const error = validateForm(event.currentTarget as HTMLFormElement);
    if (error) { fail(error); return; }
    this.appliedStatus = this.selectedStatus;
    this.appliedOrderNo = this.selectedOrderNo.trim();
    this.updateURL();
    void this.load(1);
  }

  private clearFilters() {
    if (this.tab === 'api') {
      this.usageFilters = emptyFilters();
      this.appliedUsageFilters = emptyFilters();
      this.filterError = '';
    } else {
      this.selectedStatus = this.appliedStatus = '';
      this.selectedOrderNo = this.appliedOrderNo = '';
    }
    this.updateURL();
    void this.load(1);
  }

  private changePage(event: CustomEvent<{ page: number }>) {
    const page = event.detail.page;
    if (Number.isInteger(page) && page >= 1) void this.load(page);
  }

  private async load(page: number) {
    if (!this.isConnected || !this.authorized) return;
    const tab = this.tab;
    const request = this.requests.next('list');
    this.listRequest = request;
    page = Math.max(1, Math.floor(page));
    if (tab === 'api') { this.usageLoading = true; this.usageError = ''; }
    else { this.loading = true; this.errorMessage = ''; }
    try {
      if (tab === 'api') {
        const result = await api.get<AdminUsagePage>('/admin/usage-logs' + qs({
          page, pageSize: this.usagePageSize, ...this.appliedUsageFilters, utcOffset: -new Date().getTimezoneOffset(),
        }), { signal: request });
        if (request.aborted || this.listRequest !== request || !this.isConnected || this.tab !== tab) return;
        this.usageItems = result.items;
        this.usagePage = result.page;
        this.usagePageSize = result.pageSize;
        this.usageTotal = result.total;
        this.summary = result.summary;
      } else {
        const result = await api.get<AdminNotifyPage>('/admin/notifications' + qs({
          page, pageSize: this.pageSize, status: this.appliedStatus, orderNo: this.appliedOrderNo,
        }), { signal: request });
        if (request.aborted || this.listRequest !== request || !this.isConnected || this.tab !== tab) return;
        this.items = result.items;
        this.page = result.page;
        this.pageSize = result.pageSize;
        this.total = result.total;
      }
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.isConnected && this.listRequest === request) {
        const message = error instanceof Error ? error.message : String(error);
        if (tab === 'api') this.usageError = message; else this.errorMessage = message;
      }
    } finally {
      if (this.listRequest === request) {
        this.listRequest = null;
        if (tab === 'api') this.usageLoading = false; else this.loading = false;
      }
      this.requests.release('list', request);
    }
  }

  private setBusy(id: number, pending: boolean) {
    const busy = new Set(this.busy);
    if (pending) busy.add(id); else busy.delete(id);
    this.busy = busy;
  }

  private async replay(notification: AdminNotification) {
    if (!this.isConnected || this.busy.has(notification.id) || notification.status === 1) return;
    const channel = 'replay:' + notification.id;
    const request = this.requests.next(channel);
    this.actions.set(notification.id, request);
    this.setBusy(notification.id, true);
    try {
      await api.post(`/admin/notifications/${notification.id}/replay`, undefined, { signal: request });
      if (request.aborted || !this.isConnected) return;
      ok('已重新入队，等待发送');
      if (this.tab === 'callbacks') await this.load(this.page);
      if (this.selectedCallback?.id === notification.id) await this.loadCallbackDetail(notification.id);
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.isConnected) fail(error);
    } finally {
      if (this.actions.get(notification.id) === request) { this.actions.delete(notification.id); this.setBusy(notification.id, false); }
      this.requests.release(channel, request);
    }
  }

  private openDetail(log: AdminUsageLog, event: Event) {
    this.detailOpener = event.currentTarget as HTMLElement;
    this.copiedValue = this.copyError = '';
    this.selectedLog = log;
  }

  private openCallback(notification: AdminNotification, event: Event) {
    this.detailOpener = event.currentTarget as HTMLElement;
    this.selectedLog = null;
    this.selectedCallback = notification;
    this.callbackDetail = null;
    this.callbackError = this.callbackFormError = this.copiedValue = this.copyError = '';
    this.simulationResult = null;
    void this.loadCallbackDetail(notification.id);
  }

  private async loadCallbackDetail(id: number) {
    const signal = this.requests.next('callback-detail');
    this.callbackLoading = true;
    this.callbackError = '';
    try {
      const detail = await api.get<CallbackDetail>(`/admin/notifications/${id}/detail`, { signal });
      if (signal.aborted || !this.isConnected || this.selectedCallback?.id !== id) return;
      this.callbackDetail = detail;
    } catch (error: unknown) {
      if (!isAbort(error) && !signal.aborted && this.isConnected && this.selectedCallback?.id === id) this.callbackError = error instanceof Error ? error.message : String(error);
    } finally {
      if (!signal.aborted && this.selectedCallback?.id === id) this.callbackLoading = false;
      this.requests.release('callback-detail', signal);
    }
  }

  private async simulateCallback(mode: 'duplicate' | 'delayed', event: Event) {
    event.preventDefault();
    const notification = this.selectedCallback;
    if (!notification || !this.callbackDetail?.simulationEnabled || this.busy.has(notification.id)) return;
    if (mode === 'delayed') {
      const form = this.renderRoot.querySelector<HTMLFormElement>('#callback-simulation');
      this.callbackFormError = form ? validateForm(form) ?? '' : '延迟设置不可用';
      if (this.callbackFormError) return;
    } else this.callbackFormError = '';
    const signal = this.requests.next('simulate:' + notification.id);
    this.setBusy(notification.id, true);
    try {
      const result = await api.post<SimulationResult>(`/admin/notifications/${notification.id}/simulate`, { mode, ...(mode === 'delayed' ? { delaySeconds: Number(this.callbackDelay) } : {}) }, { signal });
      if (signal.aborted || !this.isConnected) return;
      if (this.selectedCallback?.id === notification.id) this.simulationResult = result;
      ok(mode === 'duplicate' ? '重复回调已投递，可查看新记录' : '延迟回调已入队');
      if (this.tab === 'callbacks') await this.load(this.page);
    } catch (error: unknown) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) fail(error);
    } finally {
      this.setBusy(notification.id, false);
      this.requests.release('simulate:' + notification.id, signal);
    }
  }

  private viewSimulation(event: Event) {
    const result = this.simulationResult;
    if (!result || !this.selectedCallback) return;
    const current = this.selectedCallback;
    const opener = this.detailOpener;
    this.openCallback({ ...current, id: result.id, status: 0, retryCount: 0, lastError: null, lastAttemptAt: null, createdAt: result.availableAt }, event);
    this.detailOpener = opener;
  }

  private closeDetail(event?: Event) {
    event?.preventDefault();
    this.selectedLog = null;
    this.selectedCallback = this.callbackDetail = null;
    this.requests.abort('callback-detail');
    this.callbackLoading = false;
    this.callbackError = this.callbackFormError = '';
    this.simulationResult = null;
    this.copiedValue = this.copyError = '';
  }

  private backdrop(event: MouseEvent) {
    const dialog = event.currentTarget as HTMLDialogElement;
    if (event.target !== dialog) return;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) this.closeDetail();
  }

  private async copy(value: string) {
    const selected = this.selectedLog;
    const callback = this.selectedCallback;
    try {
      await navigator.clipboard.writeText(value);
      if (this.isConnected && this.selectedLog === selected && this.selectedCallback === callback) { this.copiedValue = value; this.copyError = ''; }
    } catch {
      if (this.isConnected && this.selectedLog === selected && this.selectedCallback === callback) this.copyError = '复制失败，请选择文本手动复制。';
    }
  }

  private tokenName(log: AdminUsageLog) { return log.apiKeyName || (log.apiKeyId ? `令牌 #${log.apiKeyId}` : '未识别令牌'); }
  private projectName(log: AdminUsageLog) { return log.groupName || (log.groupId ? `项目 #${log.groupId}` : '—'); }
  private maskedKey(log: Pick<AdminUsageLog, 'keyPrefix' | 'keySuffix'>) { return log.keyPrefix ? `${log.keyPrefix}…${log.keySuffix || ''}` : ''; }

  private usageRow(log: AdminUsageLog): TemplateResult {
    return html`<tr class="usage-row" data-usage-id=${log.id} tabindex="0" aria-label=${`查看 ${log.method} ${log.path} 的调用详情`}
      @click=${(event: Event) => this.openDetail(log, event)} @keydown=${(event: KeyboardEvent) => {
        if (event.target === event.currentTarget && ['Enter', ' '].includes(event.key)) { event.preventDefault(); this.openDetail(log, event); }
      }}>
      <td><div class="cell-stack"><span class=${`badge ${successful(log) ? 'ok' : 'err'}`}>${successful(log) ? '成功' : '失败'}</span><span class="result-code mono">HTTP ${log.statusCode}</span></div></td>
      <td class="endpoint-cell"><div class="cell-stack"><span class="endpoint-method">${log.method}</span><span class="mono">${log.path}</span></div></td>
      <td class="num mono">${duration(log.durationMs)}</td>
      <td class="identity-cell"><div class="cell-stack"><span>${this.tokenName(log)}</span><span class="secondary">${this.projectName(log)}</span>
        ${this.maskedKey(log) ? html`<span class="mono secondary">${this.maskedKey(log)}</span>` : nothing}</div></td>
      <td class="source-cell"><div class="cell-stack"><span class="mono">${log.clientIp || '未记录'}</span><span class="secondary">${locationSummary(log.clientIp, log.clientLocation)}</span></div></td>
      <td class="small"><time datetime=${log.createdAt}>${dateTime(log.createdAt)}</time></td>
      <td><button type="button" class="link" data-usage-detail aria-label=${`查看调用 #${log.id} 的详情`}
        @click=${(event: Event) => { event.stopPropagation(); this.openDetail(log, event); }}>详情</button></td>
    </tr>`;
  }

  private notificationRow(notification: AdminNotification): TemplateResult {
    const [label, kind] = STATUS[notification.status] ?? [String(notification.status), ''];
    const busy = this.busy.has(notification.id);
    return html`<tr>
      <td class="mono" data-order>${notification.orderNo ? html`<a href=${'/orders' + qs({ orderNo: notification.orderNo })}>${notification.orderNo}</a>` : '—'}</td>
      <td class="wrapped mono" data-url>${notification.url || '—'}</td>
      <td><span class=${kind ? `badge ${kind}` : 'badge'} data-status>${label}</span></td>
      <td class="num" data-retry>${notification.retryCount}</td>
      <td class="wrapped small err-text" data-error>${notification.lastError || '—'}</td>
      <td class="small" data-attempt>${notification.lastAttemptAt ? html`<time datetime=${notification.lastAttemptAt}>${dateTime(notification.lastAttemptAt)}</time>` : '—'}</td>
      <td class="small" data-created><time datetime=${notification.createdAt}>${dateTime(notification.createdAt)}</time></td>
      <td class="acts" data-acts><button type="button" class="link" data-callback-detail @click=${(event: Event) => this.openCallback(notification, event)}>详情</button>
        ${notification.status !== 1 ? html`<button type="button" class="link" ?disabled=${busy} @click=${() => void this.replay(notification)}>${busy ? '处理中…' : '重发'}</button>` : nothing}</td>
    </tr>`;
  }

  private renderUsage() {
    const filters = this.usageFilters;
    const tokens = filters.groupId ? this.tokens.filter((token) => String(token.groupId) === filters.groupId) : this.tokens;
    return html`<section class="usage-content" id="api-panel" role="tabpanel" aria-labelledby="api-tab">
      <div class="stats usage-stats" aria-label="调用汇总" aria-busy=${this.usageLoading}>
        <div class="stat"><span class="k">调用总数</span><span class="v" data-total-count>${this.summary ? count(this.summary.totalCount) : '—'}</span></div>
        <div class="stat"><span class="k">成功调用</span><span class="v ok-text" data-success-count>${this.summary ? count(this.summary.successCount) : '—'}</span></div>
        <div class="stat"><span class="k">失败调用</span><span class="v error-value" data-failure-count>${this.summary ? count(this.summary.failureCount) : '—'}</span></div>
        <div class="stat"><span class="k">平均耗时</span><span class="v" data-average-duration>${this.summary ? duration(this.summary.averageDurationMs) : '—'}</span></div>
      </div>
      <section class="panel usage-filter-panel"><form class="stack" id="usage-filters" novalidate @submit=${this.submitUsageFilters}>
        <div class="filter-grid">
          <div class="field keyword-field"><label for="usageKeyword">关键词</label><input id="usageKeyword" maxlength="200" placeholder="接口路径 / IP / 请求 ID / 订单号" .value=${filters.keyword}
            @input=${(event: Event) => this.setFilter('keyword', (event.currentTarget as HTMLInputElement).value)}></div>
          <div class="field"><label for="usageProject">项目</label><sp-select id="usageProject" label="项目" .value=${filters.groupId}
            .options=${[{ value: '', label: '全部项目' }, ...this.projects.map((project) => ({ value: String(project.id), label: project.name })),
              ...(filters.groupId && !this.projects.some((project) => String(project.id) === filters.groupId) ? [{ value: filters.groupId, label: `项目 #${filters.groupId}（历史记录）` }] : [])]}
            @change=${(event: Event) => this.setFilter('groupId', (event.currentTarget as SpSelect).value)}></sp-select></div>
          <div class="field"><label for="usageToken">API令牌</label><sp-select id="usageToken" label="API令牌" .value=${filters.apiKeyId}
            .options=${[{ value: '', label: '全部令牌' }, ...tokens.map((token) => ({ value: String(token.id), label: `${token.name || `令牌 #${token.id}`} · #${token.id}` })),
              ...(filters.apiKeyId && !tokens.some((token) => String(token.id) === filters.apiKeyId) ? [{ value: filters.apiKeyId, label: `令牌 #${filters.apiKeyId}（历史记录）` }] : [])]}
            @change=${(event: Event) => this.setFilter('apiKeyId', (event.currentTarget as SpSelect).value)}></sp-select></div>
          <div class="field"><label for="usageMethod">请求方法</label><sp-select id="usageMethod" label="请求方法" .value=${filters.method}
            .options=${[{ value: '', label: '全部方法' }, ...['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].map((method) => ({ value: method, label: method }))]}
            @change=${(event: Event) => this.setFilter('method', (event.currentTarget as SpSelect).value)}></sp-select></div>
          <div class="field"><label for="usageStatus">调用结果</label><sp-select id="usageStatus" label="调用结果" .value=${filters.status}
            .options=${[{ value: '', label: '全部结果' }, { value: 'success', label: '成功' }, { value: 'failure', label: '失败' }]}
            @change=${(event: Event) => this.setFilter('status', (event.currentTarget as SpSelect).value)}></sp-select></div>
          <div class="field"><label for="usageDateFrom">开始日期</label><sp-date-picker id="usageDateFrom" label="开始日期" .value=${filters.dateFrom}
            @input=${(event: Event) => this.setFilter('dateFrom', (event.currentTarget as SpDatePicker).value)}></sp-date-picker></div>
          <div class="field"><label for="usageDateTo">结束日期</label><sp-date-picker id="usageDateTo" label="结束日期" .value=${filters.dateTo}
            @input=${(event: Event) => this.setFilter('dateTo', (event.currentTarget as SpDatePicker).value)}></sp-date-picker></div>
          <div class="filter-actions"><button class="primary" type="submit">查询</button><button type="button" id="refresh" @click=${() => void this.refresh()}>刷新</button>
            <button type="button" @click=${this.clearFilters}>清除筛选</button></div>
        </div>
        ${this.filterError ? html`<p class="err-text" role="alert">${this.filterError}</p>` : nothing}
        ${this.metadataError ? html`<p class="err-text" role="status">${this.metadataError}</p>` : nothing}
      </form></section>
      <section class="panel" aria-busy=${this.usageLoading}>
        <div class="panel-head"><h2>API 调用记录</h2><span class="small muted">${count(this.usageTotal)} 条</span></div>
        <div class="table-scroll"><table class="usage-table"><thead><tr><th>结果</th><th>接口</th><th class="num">耗时</th><th>令牌 / 项目</th><th>来源 IP / 归属地</th><th>调用时间</th><th></th></tr></thead>
          <tbody id="usage-rows">${repeat(this.usageItems, (log) => log.id, (log) => this.usageRow(log))}</tbody></table></div>
        ${this.usageError ? html`<p class="err-text small" role="alert">日志加载失败：${this.usageError}</p>` : nothing}
        ${!this.usageItems.length ? html`<div class="empty" id="usage-empty" role="status">${this.usageLoading ? '正在加载使用日志…' : this.usageError ? '使用日志加载失败' : '暂无符合条件的 API 调用记录'}</div>` : nothing}
        <sp-pager .page=${this.usagePage} .pageSize=${this.usagePageSize} .total=${this.usageTotal} .busy=${this.usageLoading} @page-change=${this.changePage}></sp-pager>
      </section>
    </section>`;
  }

  private renderCallbacks() {
    return html`<section class="usage-content" id="callbacks-panel" role="tabpanel" aria-labelledby="callbacks-tab">
      <section class="panel"><form class="row" id="filters" novalidate @submit=${this.submitCallbackFilters}>
        <div class="field grow"><label for="orderNo">订单号</label><input id="orderNo" placeholder="输入完整订单号" .value=${this.selectedOrderNo}
          @input=${(event: Event) => { this.selectedOrderNo = (event.currentTarget as HTMLInputElement).value; }}></div>
        <div class="field sm"><label for="status">投递状态</label><sp-select id="status" label="投递状态" .value=${this.selectedStatus}
          .options=${[{ value: '', label: '全部' }, { value: '0', label: '待发送' }, { value: '1', label: '成功' }, { value: '2', label: '失败重试中' }, { value: '3', label: '重试耗尽' }]}
          @change=${(event: Event) => { this.selectedStatus = (event.currentTarget as SpSelect).value; }}></sp-select></div>
        <button class="primary" type="submit">查询</button><button type="button" id="refresh" @click=${() => void this.load(this.page)}>刷新</button>
        <button type="button" @click=${this.clearFilters}>清除筛选</button>
      </form>${this.appliedOrderNo ? html`<p class="filter-note">正在查看订单 ${this.appliedOrderNo} 的回调投递</p>` : nothing}</section>
      <section class="panel" aria-busy=${this.loading}>
        <div class="panel-head"><h2>回调投递记录</h2><span class="small muted">${count(this.total)} 条</span></div>
        <div class="table-scroll"><table><thead><tr><th>订单号</th><th>回调地址</th><th>状态</th><th class="num">重试</th><th>最近错误</th><th>最近尝试</th><th>创建</th><th></th></tr></thead>
          <tbody id="rows">${repeat(this.items, (notification) => notification.id, (notification) => this.notificationRow(notification))}</tbody></table></div>
        ${this.errorMessage ? html`<p class="err-text small" role="alert">${this.errorMessage}</p>` : nothing}
        ${!this.items.length ? html`<div class="empty" id="empty" role="status">${this.loading ? '正在加载回调投递…' : this.errorMessage ? '回调投递加载失败' : '暂无符合条件的回调投递记录'}</div>` : nothing}
        <sp-pager .page=${this.page} .pageSize=${this.pageSize} .total=${this.total} .busy=${this.loading} @page-change=${this.changePage}></sp-pager>
      </section>
    </section>`;
  }

  private detailField(label: string, value: string | number | TemplateResult | undefined, options: { full?: boolean; mono?: boolean; copy?: boolean } = {}) {
    const missing = value === undefined || value === '';
    return html`<div class=${options.full ? 'full' : ''}><dt>${label}</dt><dd><span class=${missing ? 'missing' : options.mono ? 'mono' : ''}>${missing ? '未记录' : value}</span>
      ${options.copy && typeof value === 'string' && value ? html`<button type="button" aria-label=${`复制${label}`} @click=${() => void this.copy(value)}>${this.copiedValue === value ? '已复制' : '复制'}</button>` : nothing}</dd></div>`;
  }

  private renderDetail() {
    const log = this.selectedLog;
    return html`<dialog class="modal usage-dialog" aria-labelledby=${this.selectedCallback ? 'callback-detail-title' : 'usage-detail-title'} @cancel=${this.closeDetail} @click=${this.backdrop}
      @close=${(event: Event) => { if (!(event.currentTarget as HTMLDialogElement).open) this.closeDetail(); }}>
      ${this.selectedCallback ? this.renderCallbackDetail() : log ? html`<div class="usage-dialog-header"><div class="cell-stack"><h3 id="usage-detail-title">API 调用详情</h3><span class="mono">${log.method} ${log.path}</span></div>
        <button type="button" data-close-detail aria-label="关闭调用详情" @click=${this.closeDetail}>关闭</button></div>
        <dl class="usage-detail-fields">
          ${this.detailField('调用结果', html`<span class=${`badge ${successful(log) ? 'ok' : 'err'}`}>${successful(log) ? '成功' : '失败'}</span>`)}
          ${this.detailField('HTTP 状态码', log.statusCode, { mono: true })}
          ${this.detailField('请求 ID', log.requestId, { full: true, mono: true, copy: true })}
          ${this.detailField('调用时间', html`<time datetime=${log.createdAt}>${dateTime(log.createdAt)}</time>`)}
          ${this.detailField('响应耗时', duration(log.durationMs))}
          ${this.detailField('项目', this.projectName(log))}${this.detailField('API令牌', this.tokenName(log))}
          ${this.detailField('项目 ID', log.groupId, { mono: true })}${this.detailField('令牌 ID', log.apiKeyId, { mono: true })}
          ${this.maskedKey(log) ? this.detailField('令牌标识', this.maskedKey(log), { full: true, mono: true }) : nothing}
          ${this.detailField('来源 IP', log.clientIp, { full: true, mono: true, copy: true })}
          ${this.detailField('IP 归属地', locationSummary(log.clientIp, log.clientLocation), { full: true })}
          ${this.detailField('国家 / 地区', cleanLocationValue(log.clientLocation?.country))}${this.detailField('省份 / 区域', cleanLocationValue(log.clientLocation?.region))}
          ${this.detailField('城市', cleanLocationValue(log.clientLocation?.city))}${this.detailField('运营商 / ISP', cleanLocationValue(log.clientLocation?.isp))}
          ${this.detailField('网络类型', log.clientIp ? networkLabel(log.clientLocation?.network) : undefined)}
          ${this.detailField('关联订单', log.orderNo ? html`<a class="mono" href=${'/orders' + qs({ orderNo: log.orderNo })}>${log.orderNo}</a>` : undefined)}
        </dl>
        ${log.errorMessage ? html`<div class="usage-error"><strong>错误信息</strong><p>${log.errorMessage}</p></div>` : nothing}
        ${this.copyError ? html`<p class="err-text" role="status">${this.copyError}</p>` : nothing}
        <div class="usage-dialog-footer"><p class="filter-note">请求来源通常为接入方服务器地址。</p>
          ${log.orderNo ? html`<a href=${'/orders' + qs({ orderNo: log.orderNo })}>查看关联订单 →</a>` : nothing}</div>` : nothing}
    </dialog>`;
  }

  private renderCallbackDetail() {
    const selected = this.selectedCallback!;
    const detail = this.callbackDetail;
    const busy = this.busy.has(selected.id);
    const [label, kind] = STATUS[detail?.status ?? selected.status] ?? ['未知状态', ''];
    return html`<div class="usage-dialog-header"><div class="cell-stack"><h3 id="callback-detail-title">回调投递详情</h3><span class="mono">回调 #${selected.id}${selected.orderNo ? ` · ${selected.orderNo}` : ''}</span></div>
      <button type="button" data-close-detail aria-label="关闭回调详情" @click=${this.closeDetail}>关闭</button></div>
      ${this.callbackLoading ? html`<p class="empty" role="status">正在加载投递详情…</p>` : nothing}
      ${this.callbackError ? html`<div class="usage-error" role="alert"><p>详情加载失败：${this.callbackError}</p><button type="button" @click=${() => void this.loadCallbackDetail(selected.id)}>重试</button></div>` : nothing}
      ${detail ? html`<dl class="usage-detail-fields">
        ${this.detailField('投递状态', html`<span class=${`badge ${kind}`}>${label}</span>`)}
        ${this.detailField('累计投递次数', detail.attempts.length)}
        ${this.detailField('回调地址', detail.url, { full: true, mono: true })}
        ${this.detailField('创建时间', dateTime(detail.createdAt))}
        ${this.detailField('最近尝试', detail.lastAttemptAt ? dateTime(detail.lastAttemptAt) : '尚未投递')}
        ${detail.nextRetryAt ? this.detailField(detail.simulationMode === 'delayed' && !detail.attempts.length ? '计划投递时间' : '下次尝试时间', dateTime(detail.nextRetryAt), { full: true }) : nothing}
        ${detail.sourceNotificationId ? this.detailField('模拟来源', html`回调 #${detail.sourceNotificationId} · ${detail.simulationMode === 'delayed' ? '延迟回调' : '重复回调'}`, { full: true }) : nothing}
      </dl>
      <section class="callback-body"><div class="panel-head"><h4>回调报文</h4><button type="button" @click=${() => void this.copy(detail.body)}>${this.copiedValue === detail.body ? '已复制' : '复制报文'}</button></div><pre class="mono">${detail.body || '未记录'}</pre></section>
      <section class="callback-history" aria-label="投递历史"><h4>投递历史</h4>
        ${detail.attempts.length ? html`<ol>${detail.attempts.map((attempt) => html`<li class="callback-attempt">
          <div class="callback-attempt-head"><strong>第 ${attempt.attemptNo} 次投递</strong><span class=${`badge ${attempt.errorMessage ? 'err' : 'ok'}`}>${attempt.httpStatus ? `HTTP ${attempt.httpStatus}` : '未收到响应'}</span><span class="muted small">${duration(attempt.durationMs)}</span></div>
          <time class="muted small" datetime=${attempt.createdAt}>${dateTime(attempt.createdAt)}</time>
          ${attempt.errorMessage ? html`<p class="err-text">${attempt.errorMessage}</p>` : nothing}
          <div class="callback-response"><span class="muted small">响应摘要</span><pre class="mono">${attempt.responseSummary || '响应为空或未收到响应正文'}</pre></div>
        </li>`)}</ol>` : html`<p class="filter-note">尚无投递记录${detail.nextRetryAt ? '，等待计划时间发送' : ''}。</p>`}
      </section>
      ${detail.simulationEnabled ? html`<section class="callback-simulation"><h4>模拟回调</h4><p class="filter-note">使用同一份报文和签名，测试接收端的重复处理和延迟处理。</p>
        <form id="callback-simulation" novalidate @submit=${(event: Event) => void this.simulateCallback('delayed', event)}>
          <button type="button" ?disabled=${busy} data-simulate-duplicate @click=${(event: Event) => void this.simulateCallback('duplicate', event)}>重复投递一次</button>
          <div class="field"><label for="callbackDelay">延迟秒数</label><input id="callbackDelay" type="number" min="1" max="3600" step="1" required .value=${this.callbackDelay} @input=${(event: Event) => { this.callbackDelay = (event.currentTarget as HTMLInputElement).value; this.callbackFormError = ''; }}></div>
          <button type="submit" ?disabled=${busy} data-simulate-delay>${busy ? '处理中…' : '创建延迟回调'}</button>
        </form>${this.callbackFormError ? html`<p class="err-text" role="alert">${this.callbackFormError}</p>` : nothing}
        ${this.simulationResult ? html`<p class="simulation-result" role="status">${this.simulationResult.mode === 'delayed' ? `计划于 ${dateTime(this.simulationResult.availableAt)} 投递` : '已尝试投递重复回调'}。<button type="button" class="link" @click=${this.viewSimulation}>查看新回调 #${this.simulationResult.id}</button></p>` : nothing}
      </section>` : nothing}
      ${this.copyError ? html`<p class="err-text" role="status">${this.copyError}</p>` : nothing}
      <div class="usage-dialog-footer"><button type="button" ?disabled=${this.callbackLoading} @click=${() => void this.loadCallbackDetail(selected.id)}>刷新详情</button>
        ${detail.status !== 1 ? html`<button type="button" ?disabled=${busy} @click=${() => void this.replay(detail)}>重新入队</button>` : nothing}
        ${detail.orderNo ? html`<a href=${'/orders' + qs({ orderNo: detail.orderNo })}>查看关联订单 →</a>` : nothing}</div>` : nothing}`;
  }

  protected override render() {
    if (!this.authorized) return nothing;
    return html`<main class="wrap">
      <div class="page-head"><h1>使用日志</h1><p>查看 API 调用结果与来源，跟踪支付回调的投递情况。</p></div>
      <div class="usage-tabs" role="tablist" aria-label="日志类型" @keydown=${this.tabKey}>
        <button type="button" role="tab" id="api-tab" aria-controls="api-panel" aria-selected=${this.tab === 'api'} tabindex=${this.tab === 'api' ? 0 : -1} @click=${() => this.switchTab('api')}>API调用</button>
        <button type="button" role="tab" id="callbacks-tab" aria-controls="callbacks-panel" aria-selected=${this.tab === 'callbacks'} tabindex=${this.tab === 'callbacks' ? 0 : -1} @click=${() => this.switchTab('callbacks')}>回调投递</button>
      </div>${this.tab === 'api' ? this.renderUsage() : this.renderCallbacks()}
    </main>${this.renderDetail()}`;
  }
}

customElements.define('sp-notifications-page', SpNotificationsPage);
