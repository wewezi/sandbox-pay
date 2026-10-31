type ValidatedControl = HTMLElement & {
  validity?: ValidityState;
  validationMessage?: string;
  labels?: NodeListOf<HTMLLabelElement> | null;
  label?: string;
  type?: string;
  min?: string;
  max?: string;
  step?: string;
  minLength?: number;
  maxLength?: number;
};

function fieldLabel(control: ValidatedControl): string {
  const labels = Array.from(control.labels ?? []).map((label) => label.textContent?.trim()).filter(Boolean);
  if (labels.length) return labels.join(' ');
  const root = control.getRootNode() as Document | ShadowRoot;
  const external = control.id ? Array.from(root.querySelectorAll<HTMLLabelElement>('label')).find((label) => label.htmlFor === control.id)?.textContent?.trim() : '';
  return external || control.closest('label')?.textContent?.trim() || control.label?.trim()
    || control.getAttribute('aria-label') || control.getAttribute('label') || '此项';
}

function validationText(control: ValidatedControl): string {
  const label = fieldLabel(control);
  const validity = control.validity;
  const type = control.type || control.localName;
  const date = type === 'date' || type === 'sp-date-picker';
  if (!validity) return `${label}填写不正确，请检查。`;
  if (validity.valueMissing) {
    const choose = ['select-one', 'select-multiple', 'select', 'checkbox', 'radio', 'file', 'sp-select', 'sp-date-picker'].includes(type);
    return `${choose ? '请选择' : '请填写'}${label}。`;
  }
  if (validity.badInput) return date ? `${label}格式不正确，请选择有效日期。` : `请输入有效的${label}。`;
  if (validity.typeMismatch) {
    if (type === 'email') return `${label}格式不正确，请输入有效的邮箱地址。`;
    if (type === 'url') return `${label}格式不正确，请输入完整的网址。`;
    return `${label}格式不正确。`;
  }
  if (validity.rangeUnderflow) return `${label}不能${date ? '早于' : '小于'} ${control.min || control.getAttribute('min') || '最小值'}。`;
  if (validity.rangeOverflow) return `${label}不能${date ? '晚于' : '大于'} ${control.max || control.getAttribute('max') || '最大值'}。`;
  if (validity.stepMismatch) return `${label}请按 ${control.step || control.getAttribute('step') || '指定'} 的间隔填写。`;
  if (validity.tooShort) return `${label}至少需要 ${control.minLength ?? control.getAttribute('minlength')} 个字符。`;
  if (validity.tooLong) return `${label}最多允许 ${control.maxLength ?? control.getAttribute('maxlength')} 个字符。`;
  if (validity.patternMismatch) return `${label}格式不正确。`;
  if (validity.customError && control.validationMessage) return control.validationMessage;
  return `${label}填写不正确，请检查。`;
}

/** Use with novalidate forms: keep browser constraints and show errors in the page. */
export function validateForm(form: HTMLFormElement): string | null {
  if (form.checkValidity()) return null;
  const invalid = Array.from(form.elements).find((control) => control.matches(':invalid') && control.localName !== 'fieldset')
    ?? Array.from(form.querySelectorAll<HTMLElement>(':invalid')).find((control) => control.localName !== 'fieldset');
  if (!(invalid instanceof HTMLElement)) return '请检查表单内容。';
  invalid.focus();
  return validationText(invalid as ValidatedControl);
}
