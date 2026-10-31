import { html, LitElement, nothing, type PropertyValues } from 'lit';
import { datePickerStyles } from '../styles/date-picker';

const weekdays = ['一', '二', '三', '四', '五', '六', '日'];
const fullWeekdays = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六'];

// Noon avoids local midnight/DST transitions. Calendar values never pass through UTC.
function localDate(year: number, month: number, day: number): Date {
  const date = new Date(0);
  date.setFullYear(year, month, day);
  date.setHours(12, 0, 0, 0);
  return date;
}

function parseDate(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const [year, month, day] = match.slice(1).map(Number);
  if (year < 1 || year > 9999) return null;
  const date = localDate(year, month - 1, day);
  return date.getFullYear() === year && date.getMonth() === month - 1 && date.getDate() === day ? date : null;
}

function formatDate(date: Date): string {
  return `${String(date.getFullYear()).padStart(4, '0')}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function todayDate(): Date {
  const date = new Date();
  return localDate(date.getFullYear(), date.getMonth(), date.getDate());
}

/** Theme-aware date input with a calendar in the browser's top layer. */
export class SpDatePicker extends LitElement {
  static formAssociated = true;
  static override shadowRootOptions: ShadowRootInit = { mode: 'open', delegatesFocus: true };
  static override properties = {
    value: { type: String }, label: { type: String }, name: { type: String, reflect: true },
    disabled: { type: Boolean, reflect: true }, required: { type: Boolean, reflect: true },
    min: { type: String }, max: { type: String },
    open: { state: true }, viewDate: { state: true }, activeDate: { state: true }, fieldsetDisabled: { state: true },
  };
  static override styles = datePickerStyles;

  declare value: string;
  declare label: string;
  declare name: string;
  declare disabled: boolean;
  declare required: boolean;
  declare min: string;
  declare max: string;
  declare private open: boolean;
  declare private viewDate: Date;
  declare private activeDate: string;
  declare private fieldsetDisabled: boolean;

  private readonly internals = this.attachInternals();
  private readonly positionSheet = new CSSStyleSheet();
  private positionRule = '';
  private customError = '';
  private opening = false;
  private openVersion = 0;
  private readonly reposition = () => this.positionCalendar();
  private readonly outsidePointer = (event: Event) => {
    if ((this.open || this.opening) && !event.composedPath().includes(this)) this.closeCalendar(false);
  };
  private readonly outsideFocus = (event: Event) => {
    if ((this.open || this.opening) && !event.composedPath().includes(this)) this.closeCalendar(false);
  };

  constructor() {
    super();
    this.value = this.label = this.name = this.min = this.max = '';
    this.disabled = this.required = this.fieldsetDisabled = this.open = false;
    this.viewDate = todayDate();
    this.activeDate = formatDate(this.viewDate);
  }

  get form() { return this.internals.form; }
  get validity() { return this.internals.validity; }
  get validationMessage() { return this.internals.validationMessage; }
  get willValidate() { return this.internals.willValidate; }
  get labels() { return this.internals.labels; }
  checkValidity() { this.syncValidity(); return this.internals.checkValidity(); }
  reportValidity() { this.syncValidity(); return this.internals.reportValidity(); }
  setCustomValidity(message: string) { this.customError = message; this.syncValidity(); }

  override focus(options?: FocusOptions) { this.control?.focus(options); }

  formDisabledCallback(disabled: boolean) { this.fieldsetDisabled = disabled; }
  formResetCallback() { this.value = this.getAttribute('value') ?? ''; this.closeCalendar(false); }
  formStateRestoreCallback(state: string | File | FormData | null) {
    this.value = typeof state === 'string' ? state : '';
  }

  override disconnectedCallback() {
    this.closeCalendar(false);
    this.removePositionListeners();
    super.disconnectedCallback();
  }

  protected override firstUpdated() {
    const root = this.renderRoot as ShadowRoot;
    root.adoptedStyleSheets = [...root.adoptedStyleSheets, this.positionSheet];
    this.syncValidity();
  }

  protected override updated(changes: PropertyValues) {
    if (changes.has('value')) {
      const date = this.clampDate(parseDate(this.value) ?? todayDate());
      this.viewDate = localDate(date.getFullYear(), date.getMonth(), 1);
      this.activeDate = formatDate(date);
    }
    if (['value', 'name', 'min', 'max', 'required', 'disabled', 'fieldsetDisabled'].some((name) => changes.has(name))) this.syncValidity();
    if (this.isDisabled && this.open) this.closeCalendar(false);
    if (this.open) this.positionCalendar();
  }

  private get isDisabled() { return this.disabled || this.fieldsetDisabled; }
  private get hasPopover() { return typeof HTMLElement.prototype.showPopover === 'function'; }
  private get control() { return this.renderRoot.querySelector<HTMLButtonElement>('#control'); }
  private get calendar() { return this.renderRoot.querySelector<HTMLElement>('#calendar'); }
  private get labelText() { return this.label || Array.from(this.labels).map((label) => label.textContent?.trim()).filter(Boolean).join(' ') || '日期'; }

  private syncValidity() {
    this.internals.setFormValue(this.isDisabled ? null : this.value, this.value);
    const control = this.control ?? undefined;
    if (this.isDisabled) { this.internals.setValidity({}); return; }
    if (this.customError) { this.internals.setValidity({ customError: true }, this.customError, control); return; }
    if (!this.value) {
      this.internals.setValidity(this.required ? { valueMissing: true } : {}, this.required ? '请选择日期' : '', control);
      return;
    }
    const date = parseDate(this.value);
    if (!date) { this.internals.setValidity({ badInput: true }, '日期格式应为 YYYY-MM-DD', control); return; }
    const minimum = parseDate(this.min);
    const maximum = parseDate(this.max);
    if (minimum && date < minimum) { this.internals.setValidity({ rangeUnderflow: true }, `日期不能早于 ${this.min}`, control); return; }
    if (maximum && date > maximum) { this.internals.setValidity({ rangeOverflow: true }, `日期不能晚于 ${this.max}`, control); return; }
    this.internals.setValidity({});
  }

  private clampDate(date: Date): Date {
    const minimum = parseDate(this.min);
    const maximum = parseDate(this.max);
    if (minimum && date < minimum) return minimum;
    if (maximum && date > maximum) return maximum;
    if (date.getFullYear() < 1) return localDate(1, 0, 1);
    if (date.getFullYear() > 9999) return localDate(9999, 11, 31);
    return date;
  }

  private isEnabled(date: Date) {
    const minimum = parseDate(this.min);
    const maximum = parseDate(this.max);
    return date.getFullYear() >= 1 && date.getFullYear() <= 9999 && (!minimum || date >= minimum) && (!maximum || date <= maximum);
  }

  private canViewMonth(offset: number) {
    const first = localDate(this.viewDate.getFullYear(), this.viewDate.getMonth() + offset, 1);
    if (first.getFullYear() < 1 || first.getFullYear() > 9999) return false;
    const last = localDate(first.getFullYear(), first.getMonth() + 1, 0);
    const minimum = parseDate(this.min);
    const maximum = parseDate(this.max);
    return (!minimum || last >= minimum) && (!maximum || first <= maximum);
  }

  private async openCalendar() {
    if (this.isDisabled || this.open || this.opening || !this.isConnected) return;
    const version = ++this.openVersion;
    this.opening = true;
    const date = this.clampDate(parseDate(this.value) ?? todayDate());
    this.viewDate = localDate(date.getFullYear(), date.getMonth(), 1);
    this.activeDate = formatDate(date);
    this.addPositionListeners();
    try {
      await this.updateComplete;
      if (version !== this.openVersion || this.isDisabled || !this.isConnected) return;
      const calendar = this.calendar;
      if (!calendar) return;
      if (this.hasPopover) calendar.showPopover();
      else calendar.setAttribute('data-fallback-open', '');
      this.open = true;
      this.positionCalendar();
      await this.updateComplete;
      if (version === this.openVersion && this.open && !this.isDisabled && this.isConnected) this.focusActiveDay();
    } finally {
      if (version === this.openVersion) {
        this.opening = false;
        if (!this.open) this.removePositionListeners();
      }
    }
  }

  private closeCalendar(restoreFocus: boolean) {
    this.openVersion++;
    this.opening = false;
    const calendar = this.calendar;
    if (this.hasPopover && calendar?.matches(':popover-open')) calendar.hidePopover();
    calendar?.removeAttribute('data-fallback-open');
    this.open = false;
    this.removePositionListeners();
    if (restoreFocus && !this.isDisabled) this.control?.focus();
  }

  private toggleCalendar() { if (this.open || this.opening) this.closeCalendar(true); else void this.openCalendar(); }
  private popoverToggle(event: Event) {
    const open = (event as ToggleEvent).newState === 'open';
    this.open = open;
    if (!open) { this.openVersion++; this.opening = false; this.removePositionListeners(); }
  }

  private addPositionListeners() {
    window.addEventListener('resize', this.reposition);
    window.addEventListener('scroll', this.reposition, true);
    window.visualViewport?.addEventListener('resize', this.reposition);
    window.visualViewport?.addEventListener('scroll', this.reposition);
    document.addEventListener('pointerdown', this.outsidePointer, true);
    document.addEventListener('focusin', this.outsideFocus, true);
  }

  private removePositionListeners() {
    window.removeEventListener('resize', this.reposition);
    window.removeEventListener('scroll', this.reposition, true);
    window.visualViewport?.removeEventListener('resize', this.reposition);
    window.visualViewport?.removeEventListener('scroll', this.reposition);
    document.removeEventListener('pointerdown', this.outsidePointer, true);
    document.removeEventListener('focusin', this.outsideFocus, true);
  }

  private positionCalendar() {
    const calendar = this.calendar;
    const control = this.control;
    if (!this.open || !calendar || !control) return;
    const anchor = control.getBoundingClientRect();
    const popup = calendar.getBoundingClientRect();
    const viewport = window.visualViewport;
    const leftEdge = viewport?.offsetLeft ?? 0;
    const topEdge = viewport?.offsetTop ?? 0;
    const width = viewport?.width ?? window.innerWidth;
    const height = viewport?.height ?? window.innerHeight;
    const left = Math.max(leftEdge + 8, Math.min(anchor.left, leftEdge + width - popup.width - 8));
    const below = anchor.bottom + 6;
    const above = anchor.top - popup.height - 6;
    const preferred = below + popup.height <= topEdge + height - 8 ? below : above;
    const top = Math.max(topEdge + 8, Math.min(preferred, topEdge + height - popup.height - 8));
    const rule = `.calendar { left: ${Math.round(left)}px; top: ${Math.round(top)}px; }`;
    if (rule !== this.positionRule) { this.positionSheet.replaceSync(rule); this.positionRule = rule; }
  }

  private focusActiveDay() {
    this.renderRoot.querySelector<HTMLButtonElement>(`button[data-date="${this.activeDate}"]`)?.focus();
  }

  private commit(value: string) {
    if (this.isDisabled || (value && (!parseDate(value) || !this.isEnabled(parseDate(value)!)))) return;
    const changed = this.value !== value;
    this.value = value;
    this.syncValidity();
    this.closeCalendar(true);
    if (changed) {
      this.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
      this.dispatchEvent(new Event('change', { bubbles: true, composed: true }));
    }
  }

  private async shiftMonth(offset: number, focusDay = false) {
    if (!this.canViewMonth(offset)) return;
    const first = localDate(this.viewDate.getFullYear(), this.viewDate.getMonth() + offset, 1);
    const previous = parseDate(this.activeDate) ?? first;
    const lastDay = localDate(first.getFullYear(), first.getMonth() + 1, 0).getDate();
    const date = this.clampDate(localDate(first.getFullYear(), first.getMonth(), Math.min(previous.getDate(), lastDay)));
    this.viewDate = first;
    this.activeDate = formatDate(date);
    await this.updateComplete;
    if (focusDay && this.open) this.focusActiveDay();
  }

  private async moveDay(offset: number) {
    const previous = parseDate(this.activeDate) ?? this.viewDate;
    const date = this.clampDate(localDate(previous.getFullYear(), previous.getMonth(), previous.getDate() + offset));
    this.viewDate = localDate(date.getFullYear(), date.getMonth(), 1);
    this.activeDate = formatDate(date);
    await this.updateComplete;
    if (this.open) this.focusActiveDay();
  }

  private controlKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && (this.open || this.opening)) { event.preventDefault(); event.stopPropagation(); this.closeCalendar(true); }
    else if (['ArrowDown', 'ArrowUp'].includes(event.key)) { event.preventDefault(); void this.openCalendar(); }
    else if (['Delete', 'Backspace'].includes(event.key) && this.value) { event.preventDefault(); this.commit(''); }
  }

  private calendarKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); this.closeCalendar(true); return; }
    if (!(event.target instanceof HTMLElement) || !event.target.closest('.day')) return;
    const date = parseDate(this.activeDate) ?? this.viewDate;
    const offsets: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7, Home: -((date.getDay() + 6) % 7), End: 6 - ((date.getDay() + 6) % 7) };
    if (event.key in offsets) { event.preventDefault(); void this.moveDay(offsets[event.key]); }
    else if (event.key === 'PageUp' || event.key === 'PageDown') { event.preventDefault(); void this.shiftMonth((event.key === 'PageUp' ? -1 : 1) * (event.shiftKey ? 12 : 1), true); }
  }

  private calendarBlur(event: FocusEvent) {
    if (event.relatedTarget instanceof Node && !this.calendar?.contains(event.relatedTarget)) this.closeCalendar(false);
  }

  private renderWeek(start: Date, row: number, today: string) {
    return html`<tr>${weekdays.map((_, column) => {
      const date = localDate(start.getFullYear(), start.getMonth(), start.getDate() + row * 7 + column);
      const value = formatDate(date);
      const selected = this.value === value;
      const enabled = this.isEnabled(date);
      return html`<td role="gridcell" aria-selected=${selected}><button type="button"
        class=${`day${date.getMonth() !== this.viewDate.getMonth() ? ' other-month' : ''}${selected ? ' selected' : ''}${value === today ? ' today' : ''}`}
        data-date=${value} tabindex=${this.activeDate === value && enabled ? 0 : -1} ?disabled=${!enabled}
        aria-label=${`${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日，${fullWeekdays[date.getDay()]}`}
        aria-current=${value === today ? 'date' : nothing} @click=${() => this.commit(value)}>${date.getDate()}</button></td>`;
    })}</tr>`;
  }

  protected override render() {
    const first = localDate(this.viewDate.getFullYear(), this.viewDate.getMonth(), 1);
    const start = localDate(first.getFullYear(), first.getMonth(), 1 - ((first.getDay() + 6) % 7));
    const today = todayDate();
    const month = `${first.getFullYear()}年 ${first.getMonth() + 1}月`;
    return html`<button id="control" class="control" type="button" ?disabled=${this.isDisabled}
      aria-label=${`${this.labelText}，${this.value || '未选择日期'}`} aria-haspopup="dialog" aria-expanded=${this.open} aria-controls="calendar"
      @click=${this.toggleCalendar} @keydown=${this.controlKeydown}>
      <span class=${this.value ? 'value' : 'value placeholder'}>${this.value ? this.value.replaceAll('-', '/') : '选择日期'}</span>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="16" rx="2"></rect><path d="M16 3v4M8 3v4M3 11h18"></path><path d="M8 15h2M14 15h2M8 18h2"></path></svg>
    </button>
    <div id="calendar" class="calendar" popover="auto" role="dialog" aria-label=${`选择${this.labelText}`} @toggle=${this.popoverToggle} @keydown=${this.calendarKeydown} @focusout=${this.calendarBlur}>
      <div class="month"><button type="button" aria-label="上个月" ?disabled=${!this.canViewMonth(-1)} @click=${() => void this.shiftMonth(-1)}><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m14 6-6 6 6 6"></path></svg></button>
        <span class="month-title" aria-live="polite" aria-atomic="true">${month}</span>
        <button type="button" aria-label="下个月" ?disabled=${!this.canViewMonth(1)} @click=${() => void this.shiftMonth(1)}><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m10 6 6 6-6 6"></path></svg></button></div>
      <table class="days" role="grid" aria-label=${month}><thead><tr>${weekdays.map((day) => html`<th scope="col">${day}</th>`)}</tr></thead>
        <tbody>${Array.from({ length: 6 }, (_, row) => this.renderWeek(start, row, formatDate(today)))}</tbody></table>
      <div class="footer"><button type="button" @click=${() => this.commit('')} ?disabled=${!this.value}>清除</button>
        <button type="button" ?disabled=${!this.isEnabled(today)} @click=${() => this.commit(formatDate(today))}>今天</button></div>
    </div>`;
  }
}

customElements.define('sp-date-picker', SpDatePicker);

declare global {
  interface HTMLElementTagNameMap { 'sp-date-picker': SpDatePicker; }
}
