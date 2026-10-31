import { LitElement, html, nothing, type PropertyValues } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { selectStyles } from '../styles/select';

export type SelectOption = { value: string; label: string; disabled?: boolean };

/** A form-associated select with an accessible, theme-aware popup. */
export class SpSelect extends LitElement {
  static formAssociated = true;
  static override shadowRootOptions = { ...LitElement.shadowRootOptions, delegatesFocus: true };
  static override properties = {
    value: { type: String }, options: { attribute: false }, label: { type: String },
    disabled: { type: Boolean, reflect: true }, required: { type: Boolean, reflect: true },
    name: { type: String, reflect: true }, open: { state: true }, activeIndex: { state: true },
    formDisabled: { state: true },
  };
  static override styles = selectStyles;

  declare value: string;
  declare options: readonly SelectOption[];
  declare label: string;
  declare disabled: boolean;
  declare required: boolean;
  declare name: string;
  declare private open: boolean;
  declare private activeIndex: number;
  declare private formDisabled: boolean;

  private readonly internals = this.attachInternals();
  private readonly positionSheet = new CSSStyleSheet();
  private resizeObserver?: ResizeObserver;
  private positionFrame = 0;
  private openVersion = 0;
  private initialValue = '';
  private customValidationMessage = '';
  private search = '';
  private searchTimer?: number;

  constructor() {
    super();
    this.value = '';
    this.options = [];
    this.label = '';
    this.disabled = this.required = this.open = this.formDisabled = false;
    this.name = '';
    this.activeIndex = -1;
  }

  get form() { return this.internals.form; }
  get validity() { return this.internals.validity; }
  get validationMessage() { return this.internals.validationMessage; }
  get willValidate() { return this.internals.willValidate; }
  get selectedIndex() { return this.options.findIndex((option) => option.value === this.value); }
  private get unavailable() { return this.disabled || this.formDisabled; }
  private get trigger() { return this.renderRoot.querySelector<HTMLButtonElement>('.trigger'); }
  private get popup() { return this.renderRoot.querySelector<HTMLElement>('.listbox'); }
  private get accessibleLabel() {
    return this.label || Array.from(this.internals.labels).map((label) => label.textContent?.trim()).filter(Boolean).join(' ') || '请选择';
  }

  override connectedCallback() {
    super.connectedCallback();
    document.addEventListener('pointerdown', this.outside, true);
    this.addEventListener('focusout', this.focusOut);
  }

  override disconnectedCallback() {
    this.close();
    document.removeEventListener('pointerdown', this.outside, true);
    this.removeEventListener('focusout', this.focusOut);
    window.clearTimeout(this.searchTimer);
    this.search = '';
    super.disconnectedCallback();
  }

  protected override firstUpdated() {
    this.initialValue = this.value;
    const root = this.renderRoot as ShadowRoot;
    root.adoptedStyleSheets = [...root.adoptedStyleSheets, this.positionSheet];
  }

  protected override updated(changed: PropertyValues) {
    this.syncForm();
    if (this.unavailable) { this.close(); return; }
    if (!this.open) return;
    if (changed.has('options')) {
      const previous = changed.get('options') as readonly SelectOption[] | undefined;
      const previousValue = previous?.[this.activeIndex]?.value;
      const nextIndex = this.options.findIndex((option) => option.value === previousValue && !option.disabled);
      this.activeIndex = nextIndex >= 0 ? nextIndex : this.startIndex();
    } else if (changed.has('value')) {
      this.activeIndex = this.startIndex();
    }
    this.schedulePosition();
    if (changed.has('activeIndex') || changed.has('options')) void this.scrollActive();
  }

  override focus(options?: FocusOptions) { this.trigger?.focus(options); }
  override blur() { this.trigger?.blur(); }
  checkValidity() { this.syncForm(); return this.internals.checkValidity(); }
  reportValidity() { this.syncForm(); return this.internals.reportValidity(); }
  setCustomValidity(message: string) { this.customValidationMessage = message; this.syncForm(); }

  formDisabledCallback(disabled: boolean) { this.formDisabled = disabled; }
  formResetCallback() { this.value = this.initialValue; this.syncForm(); this.close(); }
  formStateRestoreCallback(state: File | string | null) {
    if (typeof state === 'string') this.value = state;
    this.syncForm();
    this.close();
  }

  private syncForm() {
    this.internals.setFormValue(this.unavailable ? null : this.value, this.value);
    const missing = this.required && !this.value;
    const message = this.customValidationMessage || (missing ? `请选择${this.accessibleLabel}` : '');
    this.internals.setValidity(message ? {
      ...(this.customValidationMessage ? { customError: true } : {}),
      ...(missing ? { valueMissing: true } : {}),
    } : {}, message, this.trigger ?? undefined);
  }

  private readonly outside = (event: PointerEvent) => {
    if (this.open && !event.composedPath().includes(this)) this.close();
  };
  private readonly focusOut = () => {
    queueMicrotask(() => { if (this.open && !this.matches(':focus-within')) this.close(); });
  };
  private readonly schedulePosition = () => {
    if (!this.open || this.positionFrame) return;
    this.positionFrame = requestAnimationFrame(() => {
      this.positionFrame = 0;
      if (this.open) this.positionPopup();
    });
  };

  private startIndex(fromEnd = false) {
    const selected = this.selectedIndex;
    if (selected >= 0 && !this.options[selected].disabled) return selected;
    return fromEnd ? this.lastEnabled() : this.options.findIndex((option) => !option.disabled);
  }

  private lastEnabled() {
    for (let index = this.options.length - 1; index >= 0; index--) if (!this.options[index].disabled) return index;
    return -1;
  }

  private async showPopup(index = this.startIndex()) {
    if (this.unavailable || !this.isConnected) return;
    const version = ++this.openVersion;
    this.activeIndex = index;
    this.open = true;
    await this.updateComplete;
    if (version !== this.openVersion || !this.open || this.unavailable || !this.isConnected) return;
    const popup = this.popup;
    if (!popup) return;
    this.positionPopup();
    if (!this.open) return;
    if (typeof popup.showPopover === 'function') {
      try { popup.showPopover(); }
      catch { this.close(); return; }
    } else {
      popup.setAttribute('data-fallback-open', '');
    }
    this.positionPopup();
    this.focus({ preventScroll: true });
    window.addEventListener('resize', this.schedulePosition);
    window.addEventListener('scroll', this.schedulePosition, true);
    window.visualViewport?.addEventListener('resize', this.schedulePosition);
    window.visualViewport?.addEventListener('scroll', this.schedulePosition);
    this.resizeObserver = new ResizeObserver(this.schedulePosition);
    this.resizeObserver.observe(this);
    void this.scrollActive();
  }

  private close(restoreFocus = false) {
    this.openVersion++;
    const popup = this.popup;
    if (popup && typeof popup.hidePopover === 'function' && popup.matches(':popover-open')) popup.hidePopover();
    popup?.removeAttribute('data-fallback-open');
    this.open = false;
    this.activeIndex = -1;
    this.stopPositioning();
    if (restoreFocus && !this.unavailable && this.isConnected) this.focus({ preventScroll: true });
  }

  private stopPositioning() {
    window.removeEventListener('resize', this.schedulePosition);
    window.removeEventListener('scroll', this.schedulePosition, true);
    window.visualViewport?.removeEventListener('resize', this.schedulePosition);
    window.visualViewport?.removeEventListener('scroll', this.schedulePosition);
    this.resizeObserver?.disconnect();
    this.resizeObserver = undefined;
    cancelAnimationFrame(this.positionFrame);
    this.positionFrame = 0;
  }

  private nativeToggle(event: ToggleEvent) {
    if (event.newState !== 'closed') return;
    this.openVersion++;
    this.open = false;
    this.activeIndex = -1;
    this.stopPositioning();
  }

  private positionPopup() {
    const trigger = this.trigger;
    const popup = this.popup;
    if (!trigger || !popup) return;
    const bounds = trigger.getBoundingClientRect();
    const viewport = window.visualViewport;
    const screenLeft = viewport?.offsetLeft ?? 0;
    const screenTop = viewport?.offsetTop ?? 0;
    const screenWidth = viewport?.width ?? document.documentElement.clientWidth;
    const screenBottom = screenTop + (viewport?.height ?? window.innerHeight);
    if (!bounds.width || bounds.bottom <= screenTop || bounds.top >= screenBottom) { this.close(); return; }
    const width = Math.max(0, Math.min(screenWidth - 16, Math.max(200, bounds.width)));
    const left = Math.max(screenLeft + 8, Math.min(bounds.left, screenLeft + screenWidth - width - 8));
    const below = Math.max(0, screenBottom - bounds.bottom - 14);
    const above = Math.max(0, bounds.top - screenTop - 14);
    const wanted = Math.min(320, popup.scrollHeight || this.options.length * 36 + 10);
    const upwards = below < wanted && above > below;
    const height = Math.max(40, Math.min(320, upwards ? above : below));
    const top = upwards ? Math.max(screenTop + 8, bounds.top - 6 - Math.min(wanted, height)) : bounds.bottom + 6;
    // Constructed stylesheet rules position the popup without inline style attributes.
    this.positionSheet.replaceSync(`.listbox { left: ${left}px; top: ${top}px; width: ${width}px; max-height: ${height}px; }`);
  }

  private async scrollActive() {
    await this.updateComplete;
    if (this.open && this.activeIndex >= 0) {
      this.renderRoot.querySelector<HTMLElement>(`#select-option-${this.activeIndex}`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    }
  }

  private choose(index: number) {
    const option = this.options[index];
    if (!option || option.disabled || this.unavailable) return;
    const changed = this.value !== option.value;
    this.value = option.value;
    this.syncForm();
    this.close(true);
    if (changed) {
      this.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
      this.dispatchEvent(new Event('change', { bubbles: true, composed: true }));
    }
  }

  private move(direction: number) {
    let index = this.activeIndex;
    for (let visited = 0; visited < this.options.length; visited++) {
      index += direction;
      if (index < 0 || index >= this.options.length) return;
      if (!this.options[index].disabled) { this.activeIndex = index; return; }
    }
  }

  private typeAhead(character: string) {
    window.clearTimeout(this.searchTimer);
    this.search += character.toLocaleLowerCase();
    this.searchTimer = window.setTimeout(() => { this.search = ''; }, 700);
    const repeated = this.search.split('').every((letter) => letter === this.search[0]);
    const query = repeated ? this.search[0] : this.search;
    const current = this.open ? this.activeIndex : this.selectedIndex;
    const start = query.length === 1 ? current + 1 : Math.max(0, current);
    for (let step = 0; step < this.options.length; step++) {
      const index = (start + step + this.options.length) % this.options.length;
      const option = this.options[index];
      if (!option.disabled && option.label.toLocaleLowerCase().startsWith(query)) {
        if (this.open) this.activeIndex = index;
        else this.choose(index);
        return;
      }
    }
  }

  private keyDown(event: KeyboardEvent) {
    if (this.unavailable || event.isComposing) return;
    if (event.key === 'Tab') { this.close(); return; }
    if (event.key === 'Escape') {
      if (this.open) { event.preventDefault(); event.stopPropagation(); this.close(true); }
      return;
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      if (event.altKey && event.key === 'ArrowUp') { this.close(true); return; }
      if (!this.open) void this.showPopup(this.startIndex(event.key === 'ArrowUp'));
      else this.move(event.key === 'ArrowDown' ? 1 : -1);
      return;
    }
    if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      const index = event.key === 'Home' ? this.options.findIndex((option) => !option.disabled) : this.lastEnabled();
      if (!this.open) void this.showPopup(index); else this.activeIndex = index;
      return;
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      if (!this.open) void this.showPopup();
      else if (event.key === ' ' && this.search) this.typeAhead(' ');
      else this.choose(this.activeIndex);
      return;
    }
    if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      this.typeAhead(event.key);
    }
  }

  protected override render() {
    const selected = this.options.find((option) => option.value === this.value);
    const label = selected?.label ?? (this.value || '请选择');
    const name = this.accessibleLabel;
    return html`<button class="trigger" type="button" role="combobox" popovertarget="select-options"
      aria-label=${name} aria-haspopup="listbox" aria-expanded=${this.open} aria-controls="select-options"
      aria-required=${this.required} aria-disabled=${this.unavailable}
      aria-activedescendant=${this.open && this.activeIndex >= 0 ? `select-option-${this.activeIndex}` : nothing}
      ?disabled=${this.unavailable} @keydown=${this.keyDown}
      @click=${(event: MouseEvent) => { event.preventDefault(); if (this.open) this.close(true); else void this.showPopup(); }}>
      <span class="value-label">${label}</span>
      <svg class="arrow" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m4 6 4 4 4-4"></path></svg>
    </button>
    <div class="listbox" id="select-options" role="listbox" aria-label=${name} popover="auto" ?hidden=${!this.open}
      @toggle=${this.nativeToggle} @keydown=${this.keyDown}>
      ${repeat(this.options, (option) => option.value, (option, index) => html`<button type="button" role="option"
        id=${`select-option-${index}`} class=${index === this.activeIndex ? 'option active' : 'option'}
        tabindex="-1" aria-selected=${option.value === this.value} aria-disabled=${Boolean(option.disabled)} ?disabled=${Boolean(option.disabled)}
        @mousedown=${(event: MouseEvent) => event.preventDefault()}
        @mouseenter=${() => { if (!option.disabled) this.activeIndex = index; }} @click=${() => this.choose(index)}>
        <span class="option-label">${option.label}</span>
        <svg class="check" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m3 8 3 3 7-7"></path></svg>
      </button>`)}
      ${!this.options.length ? html`<div class="empty">暂无可选项</div>` : nothing}
    </div>`;
  }
}

customElements.define('sp-select', SpSelect);

declare global {
  interface HTMLElementTagNameMap { 'sp-select': SpSelect; }
}
