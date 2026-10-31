import { LitElement, html, nothing } from 'lit';
import { api, confirmDialog, dateTime, fail, isAbort, ok, requireAuth, yuan, qs } from './app';
import type { SpSelect } from './app';
import type { AdminTestPayResult, GroupWithKeys, AdminNotifyPage } from './api-types';
import { RequestController } from './controllers/requests';
import { pageStyles, sharedStyles } from './styles/shared';
import { sandboxStyles } from './styles/sandbox';
import { validateForm } from './services/form';

type KeyOption = { id: number; name: string; groupName: string };
type PayMethod = 'native' | 'alipay' | 'paypal';
type ReceiptContext = { productName: string; externalOrderNo: string; keyLabel: string; notifyUrl: string; returnUrl: string; payMethod: PayMethod };
const PAY_METHODS = [
  { value: 'native', label: 'Mintweft · 模拟支付' },
  { value: 'alipay', label: 'Alipay · 沙箱支付' },
  { value: 'paypal', label: 'PayPal · 沙箱支付' },
];
const RECEIPT_STATUS: Record<number, { title: string; label: string; description: string }> = {
  0: { title: '订单待支付', label: '待支付', description: '订单参数已保存，打开收银台发起支付。' },
  1: { title: '订单已支付', label: '已支付', description: '这笔订单已有付款记录，可查看支付凭证。' },
  2: { title: '订单支付失败', label: '支付失败', description: '这笔订单已记录支付失败，可查看订单详情。' },
  3: { title: '订单退款中', label: '退款中', description: '这笔订单已有退款申请，可查看处理记录。' },
  4: { title: '订单已退款', label: '已退款', description: '这笔订单已有退款记录，可查看订单详情。' },
  '-1': { title: '订单已过期', label: '已过期', description: '这笔订单已超过付款期限，可查看记录或创建新的测试订单。' },
};
type SandboxTask = { id: number; orderNo: string; scenario: string; status: string; runAt: string; createdAt: string; lastError?: string };
const SCENARIOS = [
  { value: 'paid', label: '支付成功' }, { value: 'failed', label: '支付失败' },
  { value: 'expired', label: '立即到期' }, { value: 'delayed_paid', label: '延迟到账' },
  { value: 'refund_failed', label: '退款失败' }, { value: 'refund_pending', label: '退款处理中' },
  { value: 'callback_duplicate', label: '重复回调' }, { value: 'callback_delayed', label: '延迟回调' },
];
const TASK_STATUS: Record<string, string> = { pending: '等待执行', running: '执行中', completed: '已完成', failed: '执行失败', cancelled: '已取消' };

class SpSandboxPage extends LitElement {
  static override styles = [sharedStyles, pageStyles, sandboxStyles];
  static override properties = {
    authenticated: { state: true }, productName: { state: true }, amount: { state: true },
    payMethod: { state: true }, expireMinutes: { state: true },
    externalOrderNo: { state: true }, apiKeyId: { state: true }, notifyUrl: { state: true }, returnUrl: { state: true },
    keyOptions: { state: true }, keysLoading: { state: true }, keysError: { state: true },
    paying: { state: true }, resetting: { state: true }, resetConfirmed: { state: true },
    result: { state: true }, receipt: { state: true }, errorMessage: { state: true },
    scenarioOrderNo: { state: true }, scenarioKind: { state: true }, scenarioDelay: { state: true }, scenarioAmount: { state: true },
    scenarioBusy: { state: true }, scenarioError: { state: true }, tasks: { state: true }, tasksLoading: { state: true }, tasksError: { state: true },
    accountLoaded: { state: true }, isSuperAdmin: { state: true }, allowDefaultProject: { state: true },
    sandboxEnabled: { state: true },
  };

  declare authenticated: boolean;
  declare productName: string;
  declare amount: string;
  declare payMethod: PayMethod;
  declare expireMinutes: string;
  declare externalOrderNo: string;
  declare apiKeyId: string;
  declare notifyUrl: string;
  declare returnUrl: string;
  declare keyOptions: KeyOption[];
  declare keysLoading: boolean;
  declare keysError: string;
  declare paying: boolean;
  declare resetting: boolean;
  declare resetConfirmed: boolean;
  declare result: AdminTestPayResult | null;
  declare receipt: ReceiptContext | null;
  declare errorMessage: string;
  declare scenarioOrderNo: string;
  declare scenarioKind: string;
  declare scenarioDelay: string;
  declare scenarioAmount: string;
  declare scenarioBusy: boolean;
  declare scenarioError: string;
  declare tasks: SandboxTask[];
  declare tasksLoading: boolean;
  declare tasksError: string;
  declare accountLoaded: boolean;
  declare isSuperAdmin: boolean;
  declare allowDefaultProject: boolean;
  declare sandboxEnabled: boolean;
  private taskTimer: ReturnType<typeof setInterval> | null = null;
  private payRequest: AbortSignal | null = null;
  private keysRequest: AbortSignal | null = null;
  private readonly requests = new RequestController(this, () => { void this.loadAccount(); void this.loadKeys(); void this.loadTasks(); });

  constructor() {
    super();
    this.authenticated = false;
    this.productName = 'Mintweft Test Order';
    this.amount = '0.01';
    this.payMethod = 'native';
    this.expireMinutes = '30';
    this.externalOrderNo = '';
    this.apiKeyId = '';
    this.notifyUrl = '';
    this.returnUrl = '';
    this.keyOptions = [];
    this.keysLoading = false;
    this.keysError = '';
    this.paying = false;
    this.resetting = false;
    this.resetConfirmed = false;
    this.result = null;
    this.receipt = null;
    this.errorMessage = '';
    this.scenarioOrderNo = '';
    this.scenarioKind = 'paid';
    this.scenarioDelay = '30';
    this.scenarioAmount = '';
    this.scenarioBusy = this.tasksLoading = false;
    this.scenarioError = this.tasksError = '';
    this.tasks = [];
    this.accountLoaded = this.isSuperAdmin = this.allowDefaultProject = false;
    this.sandboxEnabled = false;
  }

  override connectedCallback() {
    super.connectedCallback();
    this.authenticated = requireAuth();
    if (this.authenticated) { void this.loadAccount(); void this.loadKeys(); void this.loadTasks(); }
    this.taskTimer = setInterval(() => { if (!document.hidden && this.tasks.some(task => task.status === 'pending' || task.status === 'running')) void this.loadTasks(); }, 3000);
  }

  override disconnectedCallback() {
    this.payRequest = null;
    this.keysRequest = null;
    this.paying = false;
    this.resetting = false;
    this.resetConfirmed = false;
    this.scenarioBusy = this.tasksLoading = false;
    if (this.taskTimer) clearInterval(this.taskTimer);
    this.taskTimer = null;
    super.disconnectedCallback();
  }

  private async loadKeys() {
    if (!this.authenticated || !this.isConnected) return;
    const signal = this.requests.next('keys');
    this.keysRequest = signal;
    this.keysLoading = true;
    this.keysError = '';
    try {
      const groups = await api.get<GroupWithKeys[]>('/admin/groups', { signal });
      if (signal.aborted || this.keysRequest !== signal || !this.isConnected) return;
      this.keyOptions = groups.filter((group) => group.isActive).flatMap((group) =>
        group.keys.filter((key) => key.isActive).map((key) => ({ id: key.id, name: key.name || `令牌 #${key.id}`, groupName: group.name })));
      if (this.apiKeyId && !this.keyOptions.some((key) => String(key.id) === this.apiKeyId)) {
        this.apiKeyId = '';
      }
      if (this.accountLoaded && !this.allowDefaultProject && !this.apiKeyId && this.keyOptions.length) this.apiKeyId = String(this.keyOptions[0].id);
    } catch (error) {
      if (!isAbort(error) && this.keysRequest === signal && this.isConnected) {
        this.keysError = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (this.keysRequest === signal) {
        this.keysRequest = null;
        this.keysLoading = false;
      }
      this.requests.release('keys', signal);
    }
  }

  private async loadAccount() {
    if (!this.authenticated || !this.isConnected) return;
    const signal = this.requests.next('account');
    try {
      const [account, settings] = await Promise.all([
        api.get<{ role: string; sandboxDefaultProjectAllowed?: boolean }>('/auth/session', { signal }),
        api.get<{ sandboxEnabled: boolean }>('/admin/developer', { signal }),
      ]);
      if (signal.aborted || !this.isConnected) return;
      this.isSuperAdmin = account.role === 'SuperAdmin';
      this.allowDefaultProject = this.isSuperAdmin || account.sandboxDefaultProjectAllowed === true;
      this.accountLoaded = true;
      this.sandboxEnabled = settings.sandboxEnabled;
      if (!this.allowDefaultProject && !this.apiKeyId && this.keyOptions.length) this.apiKeyId = String(this.keyOptions[0].id);
    } catch (error) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) {
        this.isSuperAdmin = false;
        this.accountLoaded = this.allowDefaultProject = false;
        this.errorMessage = '账户权限读取失败，请刷新重试。';
      }
    } finally { this.requests.release('account', signal); }
  }

  private input(key: 'productName' | 'amount' | 'expireMinutes' | 'externalOrderNo' | 'notifyUrl' | 'returnUrl', event: Event) {
    const control = event.currentTarget as HTMLInputElement;
    control.setCustomValidity('');
    this[key] = control.value;
  }

  private validateOrderFields(form: HTMLFormElement) {
    const encoder = new TextEncoder();
    for (const [id, label] of [['productName', '商品名称'], ['externalOrderNo', '商户订单号']]) {
      const control = form.querySelector<HTMLInputElement>('#' + id);
      if (!control) continue;
      const value = control.value.trim();
      control.setCustomValidity(id === 'productName' && !value ? '请填写商品名称。'
        : encoder.encode(value).length > 200 ? `${label}过长，请缩短至 200 字节以内（中文通常每字 3 字节）。` : '');
    }
    for (const [id, label] of [['notifyUrl', '异步通知地址'], ['returnUrl', '支付返回地址']]) {
      const control = form.querySelector<HTMLInputElement>('#' + id);
      if (!control) continue;
      const value = control.value.trim();
      let valid = !value;
      if (value) {
        try {
          const url = new URL(value);
          valid = ['http:', 'https:'].includes(url.protocol) && Boolean(url.hostname);
        } catch { /* Show the field error below. */ }
      }
      control.setCustomValidity(valid ? '' : `${label}请填写完整的 HTTP 或 HTTPS 地址。`);
    }
    return validateForm(form);
  }

  private selectKey(event: Event) {
    this.apiKeyId = (event.currentTarget as SpSelect).value;
  }

  private selectPayMethod(event: Event) {
    this.payMethod = (event.currentTarget as SpSelect).value as PayMethod;
  }

  private newOrder() {
    this.result = null;
    this.receipt = null;
    this.externalOrderNo = '';
    this.errorMessage = '';
    void this.updateComplete.then(() => this.renderRoot.querySelector<HTMLInputElement>('#productName')?.focus());
  }

  private async loadTasks() {
    if (!this.authenticated || !this.isConnected) return;
    const signal = this.requests.next('scenarios');
    this.tasksLoading = true;
    try {
      const tasks = await api.get<SandboxTask[]>('/admin/sandbox/scenarios', { signal });
      if (!signal.aborted && this.isConnected) { this.tasks = tasks; this.tasksError = ''; }
    } catch (error) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) this.tasksError = error instanceof Error ? error.message : String(error);
    } finally {
      if (!signal.aborted && this.isConnected) this.tasksLoading = false;
      this.requests.release('scenarios', signal);
    }
  }

  private async runScenario(event: SubmitEvent) {
    event.preventDefault();
    if (this.scenarioBusy || this.paying || this.resetting || !this.accountLoaded || !this.sandboxEnabled) return;
    this.scenarioError = validateForm(event.currentTarget as HTMLFormElement) || '';
    if (this.scenarioError) return;
    const signal = this.requests.next('scenario-action');
    this.scenarioBusy = true;
    const orderNo = this.scenarioOrderNo.trim();
    if (!orderNo) { this.scenarioError = '请填写订单号。'; this.scenarioBusy = false; this.requests.release('scenario-action', signal); return; }
    try {
      if (this.scenarioKind.startsWith('callback_')) {
        const page = await api.get<AdminNotifyPage>('/admin/notifications' + qs({ orderNo, page: 1, pageSize: 1 }), { signal });
        const notification = page.items[0];
        if (!notification) throw new Error('此订单尚无回调记录，请先配置通知地址并完成支付。');
        await api.post('/admin/notifications/' + notification.id + '/simulate', {
          mode: this.scenarioKind === 'callback_duplicate' ? 'duplicate' : 'delayed',
          ...(this.scenarioKind === 'callback_delayed' ? { delaySeconds: Number(this.scenarioDelay) } : {}),
        }, { signal });
      } else {
        await api.post('/admin/sandbox/scenarios', {
          orderNo, scenario: this.scenarioKind,
          ...(['delayed_paid', 'refund_pending'].includes(this.scenarioKind) ? { delaySeconds: Number(this.scenarioDelay) } : {}),
          ...(this.scenarioKind.startsWith('refund_') && this.scenarioAmount ? { amount: Number(this.scenarioAmount) } : {}),
        }, { signal });
      }
      if (!signal.aborted && this.isConnected) { ok('场景已执行或加入任务'); await this.loadTasks(); }
    } catch (error) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) this.scenarioError = error instanceof Error ? error.message : String(error);
    } finally {
      if (!signal.aborted && this.isConnected) this.scenarioBusy = false;
      this.requests.release('scenario-action', signal);
    }
  }

  private async cancelTask(task: SandboxTask) {
    if (this.scenarioBusy) return;
    const signal = this.requests.next('scenario-action');
    this.scenarioBusy = true;
    try {
      await api.post('/admin/sandbox/scenarios/' + task.id + '/cancel', {}, { signal });
      if (!signal.aborted && this.isConnected) { ok('场景任务已取消'); await this.loadTasks(); }
    } catch (error) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) this.scenarioError = error instanceof Error ? error.message : String(error);
    } finally {
      if (!signal.aborted && this.isConnected) this.scenarioBusy = false;
      this.requests.release('scenario-action', signal);
    }
  }

  private scenariosContent() {
    const delayed = ['delayed_paid', 'refund_pending', 'callback_delayed'].includes(this.scenarioKind);
    const refund = this.scenarioKind.startsWith('refund_');
    return html`<section class="panel scenario-panel"><div class="panel-head"><div class="section-title"><h2>场景模拟</h2><p class="section-copy">验证成功、失败、到账延迟和退款处理；支付与退款场景仅适用于自有通道。</p></div></div>
      <form class="scenario-form" novalidate @submit=${this.runScenario}>
        <div class="field scenario-order"><label for="scenarioOrder">订单号</label><input id="scenarioOrder" required maxlength="200" placeholder="创建测试订单后自动填入，或粘贴已有订单号" .value=${this.scenarioOrderNo} ?disabled=${this.scenarioBusy} @input=${(event: Event) => { this.scenarioOrderNo = (event.currentTarget as HTMLInputElement).value; this.scenarioError = ''; }}></div>
        <div class="field"><label for="scenarioKind">测试场景</label><sp-select id="scenarioKind" label="测试场景" .value=${this.scenarioKind} .options=${SCENARIOS} ?disabled=${this.scenarioBusy} @change=${(event: Event) => { this.scenarioKind = (event.currentTarget as SpSelect).value; this.scenarioError = ''; }}></sp-select></div>
        ${delayed ? html`<div class="field scenario-small"><label for="scenarioDelay">延迟（秒）</label><input id="scenarioDelay" type="number" required min="1" max="3600" step="1" .value=${this.scenarioDelay} ?disabled=${this.scenarioBusy} @input=${(event: Event) => { this.scenarioDelay = (event.currentTarget as HTMLInputElement).value; }}></div>` : nothing}
        ${refund ? html`<div class="field scenario-small"><label for="scenarioAmount">退款金额（元）</label><input id="scenarioAmount" type="number" min="0.01" step="0.01" placeholder="全部可退金额" .value=${this.scenarioAmount} ?disabled=${this.scenarioBusy} @input=${(event: Event) => { this.scenarioAmount = (event.currentTarget as HTMLInputElement).value; }}></div>` : nothing}
        <button class="primary" type="submit" ?disabled=${this.scenarioBusy || !this.accountLoaded || !this.sandboxEnabled}>${this.scenarioBusy ? '正在执行…' : '执行场景'}</button>
      </form>
      ${this.scenarioKind === 'refund_pending' ? html`<p class="field-note">立即进入退款中，延迟结束后自动完成；等待期间可取消任务。</p>` : nothing}
      ${this.scenarioKind.startsWith('callback_') ? html`<p class="field-note">复用该订单最近一条签名回调；在 <a href=${'/notifications?tab=callbacks' + (this.scenarioOrderNo ? '&orderNo=' + encodeURIComponent(this.scenarioOrderNo) : '')}>回调投递</a> 查看投递结果和响应。</p>` : nothing}
      ${this.scenarioError ? html`<p class="err-text" role="alert">${this.scenarioError}</p>` : nothing}
      <div class="panel-head"><h3>场景任务</h3><button type="button" ?disabled=${this.tasksLoading} @click=${() => void this.loadTasks()}>${this.tasksLoading ? '加载中…' : '刷新任务'}</button></div>
      ${this.tasksError ? html`<p class="err-text" role="alert">${this.tasksError}</p>` : nothing}
      ${this.tasks.length ? html`<div class="table-scroll"><table><thead><tr><th>订单号</th><th>场景</th><th>状态</th><th>执行时间</th><th>结果</th><th></th></tr></thead><tbody>${this.tasks.map(task => this.taskRow(task))}</tbody></table></div>` : html`<p class="field-note">暂无场景任务。延迟任务会在服务重启后继续执行。</p>`}
    </section>`;
  }

  private taskRow(task: SandboxTask) {
    const cancel = task.status === 'pending' || (task.status === 'failed' && task.scenario === 'refund_pending');
    const label = SCENARIOS.find(option => option.value === task.scenario)?.label || task.scenario;
    const kind = task.status === 'completed' ? 'ok' : task.status === 'failed' ? 'err' : task.status === 'pending' ? 'warn' : task.status === 'cancelled' ? '' : 'info';
    return html`<tr>
      <td class="mono"><a href=${'/orders?orderNo=' + encodeURIComponent(task.orderNo)}>${task.orderNo}</a></td>
      <td>${label}</td><td><span class=${'badge ' + kind}>${TASK_STATUS[task.status] || task.status}</span></td>
      <td class="small muted">${dateTime(task.runAt)}</td><td class=${task.lastError ? 'small err-text' : 'small muted'}>${task.lastError || '—'}</td>
      <td>${cancel ? html`<button class="link" ?disabled=${this.scenarioBusy} @click=${() => void this.cancelTask(task)}>${task.status === 'failed' ? '取消并恢复' : '取消'}</button>` : nothing}</td>
    </tr>`;
  }

  private async copyOrder() {
    if (!this.result) return;
    try {
      await navigator.clipboard.writeText(this.result.orderNo);
      if (this.isConnected) ok('订单号已复制');
    } catch {
      if (this.isConnected) fail('复制失败，可选中订单号手动复制');
    }
  }

  private receiptContent() {
    const result = this.result;
    const receipt = this.receipt;
    if (!result || !receipt) return nothing;
    const state = RECEIPT_STATUS[result.status] || { title: '订单记录', label: '状态未知', description: '请在订单详情核对本次订单状态。' };
    const received = [1, 3, 4].includes(result.status);
    const method = PAY_METHODS.find(option => option.value === receipt.payMethod)?.label || receipt.payMethod;
    const icon = result.status === 2 ? '×' : result.status === -1 ? '−' : result.status === 3 ? '↻' : result.status === 4 ? '↩' : '✓';
    const tone = result.status === 2 ? 'failed' : result.status === -1 ? 'expired' : result.status === 1 ? 'paid' : '';
    return html`
      <div class="receipt-title"><span class=${'receipt-check ' + tone} aria-hidden="true">${icon}</span><div class="section-title">
        <h2>${state.title}</h2><p class="section-copy">${state.description}</p>
      </div></div>
      <div class="receipt-number"><code>${result.orderNo}</code><button type="button" @click=${this.copyOrder}>复制订单号</button></div>
      <dl class="receipt-grid">
        <div><dt>商品</dt><dd>${receipt.productName}</dd></div>
        <div><dt>订单金额</dt><dd>${yuan(result.amount)}</dd></div>
        <div><dt>商户订单号</dt><dd>${receipt.externalOrderNo || '未填写'}</dd></div>
        <div><dt>项目与令牌</dt><dd>${receipt.keyLabel}</dd></div>
        <div><dt>${received ? '实际支付方式' : '当前支付方式'}</dt><dd>${method}</dd></div>
        <div><dt>订单状态</dt><dd>${state.label}</dd></div>
        ${result.createdAt ? html`<div><dt>下单时间</dt><dd>${dateTime(result.createdAt)}</dd></div>` : nothing}
        <div><dt>支付截止时间</dt><dd>${dateTime(result.expireAt)}</dd></div>
        <div class="full"><dt>异步通知地址</dt><dd>${receipt.notifyUrl || '未配置，本单不发送回调'}</dd></div>
        ${receipt.returnUrl ? html`<div class="full"><dt>支付返回地址</dt><dd>${receipt.returnUrl}</dd></div>` : nothing}
      </dl>
      <div class="receipt-actions">
        <a class="action-link primary" href=${result.data} target="_blank" rel="noopener">${result.status === 0 ? '打开收银台' : '查看收银台记录'}</a>
        <a class="action-link" href=${'/orders?orderNo=' + encodeURIComponent(result.orderNo)}>查看订单详情</a>
        <button type="button" @click=${this.newOrder}>再下一笔</button>
      </div>`;
  }

  private orderPreview() {
    const result = this.result;
    const receipt = this.receipt;
    const saved = Boolean(result && receipt);
    const key = this.keyOptions.find(option => String(option.id) === this.apiKeyId);
    const amount = saved ? result!.amount : Number(this.amount);
    const amountReady = saved || (this.amount.trim() !== '' && Number.isFinite(amount) && amount >= 0.01
      && Math.abs(amount - Math.round(amount * 100) / 100) < 1e-9);
    const minutes = Number(this.expireMinutes);
    const expiryReady = this.expireMinutes.trim() !== '' && Number.isInteger(minutes) && minutes >= 1 && minutes <= 120;
    const method = saved ? receipt!.payMethod : this.payMethod;
    const methodLabel = PAY_METHODS.find(option => option.value === method)?.label || method;
    const product = (saved ? receipt!.productName : this.productName).trim();
    const external = saved ? receipt!.externalOrderNo : this.externalOrderNo.trim();
    const project = saved ? result!.groupName || (result!.apiKeyId ? '项目信息未返回' : '默认项目')
      : this.apiKeyId ? key?.groupName || (this.keysLoading ? '正在读取…' : '待选择有效项目')
        : !this.accountLoaded ? '正在读取…' : this.allowDefaultProject ? '默认项目' : '待选择项目';
    const token = saved ? result!.apiKeyId ? result!.apiKeyName || `令牌 #${result!.apiKeyId}` : '未使用令牌'
      : this.apiKeyId ? key?.name || (this.keysLoading ? '正在读取…' : '待选择有效令牌')
        : !this.accountLoaded ? '正在读取…' : this.allowDefaultProject ? '未使用令牌' : '待选择令牌';
    const notify = saved ? receipt!.notifyUrl : this.notifyUrl.trim();
    const returned = saved ? receipt!.returnUrl : this.returnUrl.trim();
    const state = saved ? RECEIPT_STATUS[result!.status]?.label || '已保存' : '待创建';
    return html`<section class="panel order-preview" aria-label="订单参数摘要">
      <div class="preview-head"><h2>订单摘要</h2><span class="preview-tag">${state}</span></div>
      <div class="preview-total"><span>订单金额 · CNY</span>
        <p class=${'preview-amount' + (amountReady ? '' : ' is-empty')}>${amountReady ? yuan(amount) : '待填写金额'}</p>
        <p class=${'preview-product' + (product ? '' : ' muted')}>${product || '待填写商品名称'}</p>
      </div>
      <dl class="preview-facts">
        <div><dt>支付方式</dt><dd class="preview-method">${methodLabel}</dd></div>
        <div><dt>${saved ? '支付截止' : '有效期'}</dt><dd class="preview-expiry">${saved ? dateTime(result!.expireAt) : expiryReady ? `${minutes} 分钟` : '待填写有效期'}</dd></div>
        <div><dt>所属项目</dt><dd class="preview-project">${project}</dd></div>
        <div><dt>API 令牌</dt><dd class="preview-token">${token}</dd></div>
        <div><dt>商户单号</dt><dd class=${'preview-external' + (external ? ' mono' : ' muted')}>${external || '未填写'}</dd></div>
      </dl>
      <div class="preview-destinations">
        <div class="preview-destination"><div><span>异步通知</span><span class=${'preview-address-state' + (notify ? ' filled' : '')}>${notify ? saved ? '已配置' : '已填写' : '未填写'}</span></div>
          <p class="preview-url">${notify || '本单不发送回调'}</p></div>
        <div class="preview-destination"><div><span>支付返回</span><span class=${'preview-address-state' + (returned ? ' filled' : '')}>${returned ? saved ? '已配置' : '已填写' : '未填写'}</span></div>
          <p class="preview-url">${returned || '支付后停留在收银台'}</p></div>
      </div>
      <p class="preview-note">${saved ? '摘要显示本次下单回执，重复下单保留已有订单的参数。'
        : method === 'paypal' ? 'PayPal 以 USD 结算，实际美元报价将在收银台显示。'
          : method === 'alipay' ? '创建订单后，在收银台使用支付宝沙箱完成付款。'
            : '创建订单后，可在收银台完成 Mintweft 模拟支付。'}</p>
    </section>`;
  }

  protected override render() {
    if (!this.authenticated) return nothing;
    const busy = this.paying || this.resetting || !this.accountLoaded || !this.sandboxEnabled;
    return html`
      <main class="wrap mid">
        <div class="page-head"><h1>沙箱</h1><p>创建测试订单，验证支付、退款和回调。</p></div>
        ${this.accountLoaded && !this.sandboxEnabled ? html`<p class="field-note" role="status">沙箱模式已关闭，测试下单、场景模拟和重置数据不可用。</p>` : nothing}
        <div class="sandbox-layout">
          <section class="panel test-panel">
            ${this.result ? this.receiptContent() : html`
              <div class="section-title"><h2>测试下单</h2><p class="section-copy">填写订单、支付和回调参数，创建后到收银台发起支付。</p></div>
              <form class="test-form" id="testPay" novalidate @submit=${this.pay}>
                <div class="form-grid basics-grid">
                  <div class="field"><label for="productName">商品名称</label>
                    <input id="productName" name="productName" maxlength="200" required placeholder="请输入本次测试的商品名称" .value=${this.productName}
                      ?disabled=${busy} @input=${(event: Event) => this.input('productName', event)}>
                  </div>
                  <div class="field"><label for="amount">金额（元）</label>
                    <input id="amount" name="amount" type="number" min="0.01" step="0.01" required .value=${this.amount}
                      ?disabled=${busy} @input=${(event: Event) => this.input('amount', event)}>
                  </div>
                </div>
                <div class="form-section"><h3>支付参数</h3>
                  <div class="form-grid">
                    <div class="field"><label for="payMethod">初始支付方式</label>
                      <sp-select id="payMethod" name="payMethod" label="初始支付方式" .value=${this.payMethod} .options=${PAY_METHODS} required ?disabled=${busy} @change=${this.selectPayMethod}></sp-select>
                      <p class="field-note">在收银台中发起付款，付款前可切换其他方式。</p>
                    </div>
                    <div class="field"><label for="expireMinutes">订单有效期（分钟）</label>
                      <input id="expireMinutes" name="expireMinutes" type="number" min="1" max="120" step="1" required .value=${this.expireMinutes} ?disabled=${busy} @input=${(event: Event) => this.input('expireMinutes', event)}>
                      <p class="field-note">可设置 1–120 分钟，重复下单保留原付款期限。</p>
                    </div>
                  </div>
                  <div class="field"><label for="currency">订单币种</label><input id="currency" value="CNY · 人民币" readonly>
                    <p class="field-note">${this.payMethod === 'paypal' ? 'PayPal 按锁定汇率折算为 USD，美元报价在收银台确认。' : '订单金额以人民币计价。'}</p>
                  </div>
                </div>
                <div class="form-section"><h3>订单与接入方</h3>
                  <div class="form-grid">
                    <div class="field"><label for="externalOrderNo">商户订单号（选填）</label>
                      <input id="externalOrderNo" name="externalOrderNo" maxlength="200" placeholder="例如 DEV-ORDER-001" .value=${this.externalOrderNo}
                        ?disabled=${busy} @input=${(event: Event) => this.input('externalOrderNo', event)}>
                      <p class="field-note">同一项目、订单号、金额和支付方式会复用已有订单。</p>
                    </div>
                    <div class="field"><label for="apiKeyId">${this.allowDefaultProject ? '所属项目 / API令牌（选填）' : '所属项目 / API令牌'}</label>
                      <sp-select id="apiKeyId" name="apiKeyId" label="所属项目 / API令牌" .value=${this.apiKeyId} ?disabled=${busy || this.keysLoading} ?required=${!this.allowDefaultProject}
                        .options=${[{ value: '', label: this.allowDefaultProject ? '默认项目 · 不使用令牌' : '请选择所属项目令牌', disabled: !this.allowDefaultProject }, ...this.keyOptions.map((key) => ({ value: String(key.id), label: `${key.groupName} / ${key.name}` }))]}
                        @change=${this.selectKey}></sp-select>
                      ${this.keysLoading ? html`<p class="field-note">正在读取可用令牌…</p>` : this.keysError ? html`
                        <div class="inline-error"><span class="err-text">${this.keysError}</span><button class="link" type="button" @click=${() => void this.loadKeys()}>重试</button></div>`
                        : !this.keyOptions.length ? html`<p class="field-note">${this.allowDefaultProject ? '默认项目可直接测试，也可' : '请'}前往 <a href="/groups">API令牌</a> 创建所属项目令牌。</p>`
                          : html`<p class="field-note">选择后订单归属令牌所在项目；回调使用项目独立密钥签名。</p>`}
                    </div>
                  </div>
                </div>
                <div class="form-section"><h3>通知与返回地址</h3>
                  <div class="field"><label for="notifyUrl">异步通知地址（选填）</label>
                    <input id="notifyUrl" name="notifyUrl" type="url" placeholder="https://your-app.example/payment/notify"
                      .value=${this.notifyUrl} ?disabled=${busy} @input=${(event: Event) => this.input('notifyUrl', event)}>
                    <p class="field-note">支付和退款后发送签名回调；在 <a href="/groups">项目与回调设置</a> 查看验签密钥。</p>
                  </div>
                  <div class="field"><label for="returnUrl">支付返回地址（选填）</label>
                    <input id="returnUrl" name="returnUrl" type="url" placeholder="https://your-app.example/payment/result" .value=${this.returnUrl}
                      ?disabled=${busy} @input=${(event: Event) => this.input('returnUrl', event)}>
                  </div>
                </div>
                ${this.errorMessage ? html`<p class="err-text" role="alert">${this.errorMessage}</p>` : nothing}
                <div class="submit-row"><span>创建后保留订单号，便于追踪这次联调。</span>
                  <button class="primary" type="submit" ?disabled=${busy}>${this.paying ? '正在创建…' : '创建测试订单'}</button>
                </div>
              </form>`}
          </section>
          <aside class="sandbox-sidebar"><section class="panel"><h2>联调流程</h2><ol class="steps">
            <li><div class="step-copy"><strong>创建订单</strong><p>填写商品、支付方式、有效期和商户订单号。测试回调时填写通知地址。</p></div></li>
            <li><div class="step-copy"><strong>完成支付</strong><p>打开收银台模拟到账，或切换到已配置的支付宝、PayPal 沙箱。</p></div></li>
            <li><div class="step-copy"><strong>检查记录</strong><p>在订单详情核对渠道单号、支付时间、退款明细和通知结果。</p></div></li>
          </ol></section>${this.orderPreview()}</aside>
        </div>
        ${this.scenariosContent()}
        ${this.isSuperAdmin ? html`<section class="panel reset-panel"><div><h2>重置数据</h2><p class="section-copy">清除订单、退款记录和回调投递；保留 API 使用日志、项目、令牌和管理员。</p></div>
          <button class="danger" type="button" id="reset" ?disabled=${busy} @click=${this.reset}>
            ${this.resetConfirmed ? '正在清空…' : '清空订单与通知'}
          </button>
        </section>` : nothing}
      </main>`;
  }

  private async pay(event: SubmitEvent) {
    event.preventDefault();
    if (this.paying || this.resetting || !this.accountLoaded || !this.sandboxEnabled) return;
    this.errorMessage = this.validateOrderFields(event.currentTarget as HTMLFormElement) || '';
    if (this.errorMessage) return;
    const signal = this.requests.next('pay');
    this.payRequest = signal;
    this.paying = true;
    this.errorMessage = '';
    const key = this.keyOptions.find((option) => String(option.id) === this.apiKeyId);
    const receipt: ReceiptContext = {
      productName: this.productName.trim(), externalOrderNo: this.externalOrderNo.trim(),
      payMethod: this.payMethod,
      keyLabel: key ? `${key.groupName} / ${key.name}` : '默认项目 · 不使用令牌',
      notifyUrl: this.notifyUrl.trim(), returnUrl: this.returnUrl.trim(),
    };
    try {
      const result = await api.post<AdminTestPayResult>('/admin/test-pay', {
        amount: Number(this.amount), productName: receipt.productName,
        payMethod: this.payMethod, expireMinutes: Number(this.expireMinutes),
        ...(receipt.externalOrderNo ? { externalOrderNo: receipt.externalOrderNo } : {}),
        ...(this.apiKeyId ? { apiKeyId: Number(this.apiKeyId) } : {}),
        ...(receipt.notifyUrl ? { notifyUrl: receipt.notifyUrl } : {}),
        ...(receipt.returnUrl ? { returnUrl: receipt.returnUrl } : {}),
      }, { signal });
      if (signal.aborted || this.payRequest !== signal || !this.isConnected) return;
      if (!result.data || !result.orderNo) throw new Error('后端未返回订单号或收银台地址');
      this.result = result;
      this.scenarioOrderNo = result.orderNo;
      this.receipt = {
        productName: result.productName || receipt.productName,
        payMethod: result.payMethod || 'native',
        externalOrderNo: result.externalOrderNo || '',
        keyLabel: result.apiKeyId
          ? `${result.groupName || '项目'} / ${result.apiKeyName || `令牌 #${result.apiKeyId}`}`
          : `${result.groupName || '默认项目'} · 不使用令牌`,
        notifyUrl: result.notifyUrl || '', returnUrl: result.returnUrl || '',
      };
    } catch (error) {
      if (!isAbort(error) && this.payRequest === signal && this.isConnected) {
        this.errorMessage = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (this.payRequest === signal) {
        this.payRequest = null;
        this.paying = false;
      }
      this.requests.release('pay', signal);
    }
  }

  private async reset() {
    if (this.paying || this.resetting || this.scenarioBusy || !this.isSuperAdmin || !this.sandboxEnabled) return;
    const signal = this.requests.next('reset');
    this.resetting = true;
    try {
      const confirmed = await confirmDialog({
        title: '清空订单与通知',
        message: '所有订单、退款和回调投递记录都会被删除，不可恢复。API 使用日志、项目、令牌和管理员账号保留。',
        confirmText: '确认清空', danger: true,
      });
      if (!confirmed || signal.aborted || !this.isConnected) return;
      this.resetConfirmed = true;
      await api.post('/admin/reset-data', {}, { signal });
      if (!signal.aborted && this.isConnected) {
        this.result = null;
        this.receipt = null;
        ok('已清空');
      }
    } catch (error) {
      if (!isAbort(error) && this.isConnected) fail(error);
    } finally {
      this.requests.release('reset', signal);
      if (!signal.aborted) {
        this.resetting = false;
        this.resetConfirmed = false;
      }
    }
  }
}

customElements.define('sp-sandbox-page', SpSandboxPage);
