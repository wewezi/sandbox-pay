export function must(selector: string, root: ParentNode): HTMLElement {
  const found = root.querySelector(selector);
  if (!(found instanceof HTMLElement)) throw new Error(`找不到元素：${selector}`);
  return found;
}
export function mustBe<T extends HTMLElement>(selector: string, constructor: new () => T, root: ParentNode): T {
  const found = must(selector, root);
  if (!(found instanceof constructor)) throw new Error(`${selector} 不是 ${constructor.name}`);
  return found;
}
