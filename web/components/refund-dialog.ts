import { LitElement, css, html, nothing, type PropertyValues } from 'lit';
import { api, fail, isAbort, yuan } from '../app';
import type { AdminTransaction, RefundResult } from '../api-types';
import { sharedStyles, modalStyles } from '../styles/shared';
import { RequestController } from '../controllers/requests';
import { validateForm } from '../services/form';

class SpRefundDialog extends LitElement {
  static override properties = {
    order: { attribute: false },
    amount: { state: true },
    reason: { state: true },
    pending: { state: true },
    errorMessage: { state: true },
  };

  static override styles = [sharedStyles, modalStyles, css`
    :host { display: contents; }
    form { display: flex; flex-direction: column; gap: var(--s4); }
    .summary { margin: calc(var(--s2) * -1) 0 0; }
  `];

  declare order: AdminTransaction | null;
  declare private amount: string;
  declare private reason: string;
  declare private pending: boolean;
  declare private errorMessage: string;
  private requestId = '';
  private request: AbortSignal | null = null;
  private readonly requests = new RequestController(this);
  private settled = false;

  constructor() {
    super();
    this.order = null;
    this.amount = '';
    this.reason = '';
    this.pending = false;
    this.errorMessage = '';
  }

  private get max() {
    return this.order ? Number((this.order.amount - this.order.refundedAmount).toFixed(2)) : 0;
  }

  protected override willUpdate(changed: PropertyValues<this>) {
    if (changed.has('order')) {
      this.requests.abort('refund');
      this.request = null;
      this.requestId = crypto.randomUUID();
      this.settled = false;
      this.amount = String(this.max);
      this.reason = '';
      this.pending = false;
      this.errorMessage = '';
    }
  }

  protected override updated(changed: PropertyValues<this>) {
    if (!changed.has('order') || !this.order || !this.isConnected) return;
    const dialog = this.renderRoot.querySelector<HTMLDialogElement>('dialog');
    if (!dialog) return;
    if (!dialog.open) dialog.showModal();
    this.renderRoot.querySelector<HTMLInputElement>('#refund-amount')?.focus();
  }

  override disconnectedCallback() {
    this.request = null;
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    super.disconnectedCallback();
  }

  private finish(result: RefundResult | null = null) {
    if (this.settled || !this.isConnected) return;
    this.settled = true;
    this.renderRoot.querySelector<HTMLDialogElement>('dialog')?.close();
    this.dispatchEvent(new CustomEvent(result ? 'refund-complete' : 'refund-close', {
      detail: result, bubbles: true, composed: true,
    }));
  }

  private cancel(event?: Event) {
    event?.preventDefault();
    if (!this.pending) this.finish();
  }

  private backdrop(event: MouseEvent) {
    const dialog = event.currentTarget as HTMLDialogElement;
    if (event.target !== dialog || this.pending) return;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right ||
      event.clientY < rect.top || event.clientY > rect.bottom) this.finish();
  }

  private submit(event: SubmitEvent) {
    event.preventDefault();
    if (this.pending) return;
    this.errorMessage = validateForm(event.currentTarget as HTMLFormElement) || '';
    if (!this.errorMessage) void this.send();
  }

  private async send() {
    const order = this.order;
    const amount = Number(this.amount);
    if (!order || !this.isConnected || this.pending) return;
    if (!Number.isFinite(amount) || amount <= 0 || amount > this.max + 0.001) {
      this.errorMessage = '请输入有效的退款金额';
      return;
    }
    const request = this.requests.next('refund');
    this.request = request;
    this.pending = true;
    this.errorMessage = '';
    const reason = this.reason.trim();
    try {
      const result = await api.post<RefundResult>('/admin/refund', {
        orderNo: order.orderNo, amount, requestId: this.requestId,
        ...(reason ? { reason } : {}),
      }, { signal: request });
      if (request.aborted || this.request !== request || !this.isConnected) return;
      this.pending = false;
      this.finish(result);
    } catch (error: unknown) {
      if (!isAbort(error) && !request.aborted && this.request === request && this.isConnected) {
        this.errorMessage = error instanceof Error ? error.message : String(error);
        fail(error);
      }
    } finally {
      if (this.request === request) {
        this.request = null;
        this.pending = false;
      }
      this.requests.release('refund', request);
    }
  }

  protected override render() {
    const order = this.order;
    if (!order) return nothing;
    return html`<dialog class="modal" aria-labelledby="refund-title"
      @cancel=${this.cancel} @close=${() => this.finish()} @click=${this.backdrop}>
      <form novalidate @submit=${this.submit} @input=${() => { this.errorMessage = ''; }}>
        <h3 id="refund-title">退款</h3>
        <p class="muted small summary">${order.orderNo} · 可退 ${yuan(this.max)}</p>
        <div class="field">
          <label for="refund-amount">退款金额（元）</label>
          <input id="refund-amount" data-amount type="number" step="0.01" min="0.01" max=${this.max}
            .value=${this.amount} ?disabled=${this.pending} required
            @input=${(event: Event) => { this.amount = (event.currentTarget as HTMLInputElement).value; }}>
        </div>
        <div class="field">
          <label for="refund-reason">退款原因</label>
          <input id="refund-reason" data-reason maxlength="255" placeholder="选填"
            .value=${this.reason} ?disabled=${this.pending}
            @input=${(event: Event) => { this.reason = (event.currentTarget as HTMLInputElement).value; }}>
        </div>
        ${this.errorMessage ? html`<p class="err-text small" role="alert">${this.errorMessage}</p>` : nothing}
        <div class="acts">
          <button type="button" ?disabled=${this.pending} @click=${this.cancel}>取消</button>
          <button class="primary" type="submit" ?disabled=${this.pending}>${this.pending ? '提交中…' : '提交退款'}</button>
        </div>
      </form>
    </dialog>`;
  }
}

customElements.define('sp-refund-dialog', SpRefundDialog);
