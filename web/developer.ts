import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { api, isAbort, requireAuth, type SpSelect } from './app';
import { RequestController } from './controllers/requests';
import { validateForm } from './services/form';
import { sharedStyles, pageStyles } from './styles/shared';
import { developerStyles } from './styles/developer';

type Schema = {
  $ref?: string; type?: string | string[]; description?: string; format?: string; enum?: unknown[];
  properties?: Record<string, Schema>; items?: Schema; required?: string[]; nullable?: boolean;
  minimum?: number; maximum?: number; minLength?: number; maxLength?: number; pattern?: string;
  example?: unknown; default?: unknown; allOf?: Schema[]; oneOf?: Schema[]; anyOf?: Schema[];
};
type MediaType = { schema?: Schema; example?: unknown; examples?: Record<string, { value?: unknown }> };
type Parameter = { name: string; in: string; required?: boolean; description?: string; schema?: Schema; example?: unknown; $ref?: string };
type ResponseDoc = { description?: string; content?: Record<string, MediaType>; $ref?: string };
type Operation = { summary?: string; description?: string; tags?: string[]; parameters?: Parameter[]; requestBody?: { required?: boolean; content?: Record<string, MediaType>; $ref?: string }; responses?: Record<string, ResponseDoc>; security?: Record<string, string[]>[] };
type OpenAPI = { info?: { title?: string; version?: string; description?: string }; paths: Record<string, Record<string, unknown>>; components?: Record<string, unknown>; security?: Record<string, string[]>[] };
type DeveloperMetadata = { baseUrl: string; sandboxEnabled: boolean; openapiUrl: string; exampleUrl: string; openapi: OpenAPI };
type Endpoint = { id: string; method: string; path: string; category: string; operation: Operation; parameters: Parameter[] };
type FieldRow = { name: string; schema: Schema; required: boolean; description?: string };
type DebugResult = { status: number; durationMs: number; output: string; paymentURL?: string; truncated: boolean };
type VerificationResult = { valid: boolean; expected: string };

const METHODS = new Set(['get', 'post', 'put', 'patch', 'delete', 'head', 'options']);
const DEBUG_OPERATIONS = [
  { id: 'POST /gateway/pay', method: 'POST', path: '/gateway/pay', label: '下单 · POST /gateway/pay' },
  { id: 'GET /gateway/query', method: 'GET', path: '/gateway/query', label: '查询订单 · GET /gateway/query' },
  { id: 'POST /gateway/refund', method: 'POST', path: '/gateway/refund', label: '退款 · POST /gateway/refund' },
] as const;
const shellQuote = (value: string) => "'" + value.replace(/'/g, "'\"'\"'") + "'";
const pretty = (value: unknown) => JSON.stringify(value, null, 2) ?? '';
const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

class SpDeveloperPage extends LitElement {
  static override properties = {
    authorized: { state: true }, metadata: { state: true }, loading: { state: true }, loadError: { state: true },
    category: { state: true }, endpointID: { state: true }, responseCode: { state: true },
    debugOperation: { state: true }, queryText: { state: true }, requestBody: { state: true }, debugBusy: { state: true }, debugError: { state: true }, debugResult: { state: true },
    verifyVersion: { state: true }, verifyBody: { state: true }, verifyTimestamp: { state: true }, verifyNonce: { state: true }, verifySignature: { state: true }, verifyBusy: { state: true }, verifyError: { state: true }, verification: { state: true },
    copied: { state: true }, copyError: { state: true },
  };
  static override styles = [sharedStyles, pageStyles, developerStyles];

  declare private authorized: boolean;
  declare private metadata: DeveloperMetadata | null;
  declare private loading: boolean;
  declare private loadError: string;
  declare private category: string;
  declare private endpointID: string;
  declare private responseCode: string;
  declare private debugOperation: string;
  declare private queryText: string;
  declare private requestBody: string;
  declare private debugBusy: boolean;
  declare private debugError: string;
  declare private debugResult: DebugResult | null;
  declare private verifyVersion: string;
  declare private verifyBody: string;
  declare private verifyTimestamp: string;
  declare private verifyNonce: string;
  declare private verifySignature: string;
  declare private verifyBusy: boolean;
  declare private verifyError: string;
  declare private verification: VerificationResult | null;
  declare private copied: string;
  declare private copyError: string;
  private verifyRun = 0;
  private debugRun = 0;
  private copyRun = 0;
  private readonly requests = new RequestController(this, () => { this.clearSensitive(); void this.load(); });
  private readonly hideSensitive = () => this.clearSensitive();

  constructor() {
    super();
    this.authorized = this.loading = this.debugBusy = this.verifyBusy = false;
    this.metadata = this.debugResult = this.verification = null;
    this.loadError = this.queryText = this.debugError = this.verifyError = this.copied = this.copyError = '';
    this.category = '支付网关'; this.endpointID = this.debugOperation = 'POST /gateway/pay';
    this.responseCode = '200'; this.verifyVersion = '1';
    this.verifyBody = this.verifyTimestamp = this.verifyNonce = this.verifySignature = '';
    this.requestBody = pretty({ amount: 1, productName: '联调订单', payMethod: 'native', externalOrderNo: 'demo_order_001' });
  }

  override connectedCallback() {
    super.connectedCallback();
    this.authorized = requireAuth();
    window.addEventListener('pagehide', this.hideSensitive);
    if (this.authorized) void this.load();
  }

  override disconnectedCallback() {
    this.clearSensitive();
    window.removeEventListener('pagehide', this.hideSensitive);
    super.disconnectedCallback();
  }

  private clearSensitive() {
    for (const input of this.renderRoot.querySelectorAll<HTMLInputElement>('input[type=password]')) input.value = '';
    this.verifyRun++; this.debugRun++; this.copyRun++;
    this.verifyBusy = this.debugBusy = false;
    this.verification = null;
    this.requests.abort('debug');
  }

  private async load() {
    if (!this.isConnected || !this.authorized) return;
    const signal = this.requests.next('metadata');
    this.loading = true;
    this.loadError = '';
    try {
      const metadata = await api.get<DeveloperMetadata>('/admin/developer', { signal });
      if (signal.aborted || !this.isConnected) return;
      if (!metadata?.openapi?.paths || !isObject(metadata.openapi.paths)) throw new Error('接口文档格式不正确');
      const initial = this.metadata === null;
      this.metadata = metadata;
      const endpoints = this.endpoints;
      if (!endpoints.some((endpoint) => endpoint.id === this.endpointID)) this.endpointID = endpoints[0]?.id ?? '';
      this.category = this.endpoint?.category ?? '';
      this.responseCode = Object.keys(this.endpoint?.operation.responses ?? {})[0] ?? '200';
      if (initial) this.resetDebug(this.debugOperation);
    } catch (error: unknown) {
      if (!isAbort(error) && !signal.aborted && this.isConnected) this.loadError = error instanceof Error ? error.message : String(error);
    } finally {
      if (!signal.aborted) this.loading = false;
      this.requests.release('metadata', signal);
    }
  }

  private resolve<T>(value: T & { $ref?: string }): T {
    if (!value?.$ref?.startsWith('#/')) return value;
    let result: unknown = this.metadata?.openapi;
    for (const part of value.$ref.slice(2).split('/')) {
      if (!isObject(result)) return value;
      result = result[part.replace(/~1/g, '/').replace(/~0/g, '~')];
    }
    return isObject(result) ? result as T : value;
  }

  private get endpoints(): Endpoint[] {
    const endpoints: Endpoint[] = [];
    for (const [path, item] of Object.entries(this.metadata?.openapi.paths ?? {})) {
      for (const [method, raw] of Object.entries(item)) {
        if (!METHODS.has(method) || !isObject(raw)) continue;
        const operation = raw as Operation;
        endpoints.push({ id: `${method.toUpperCase()} ${path}`, method: method.toUpperCase(), path, category: operation.tags?.[0] || '其他接口', operation, parameters: [...(Array.isArray(item.parameters) ? item.parameters as Parameter[] : []), ...(operation.parameters ?? [])].map((parameter) => this.resolve(parameter)) });
      }
    }
    return endpoints;
  }

  private get endpoint() { return this.endpoints.find((endpoint) => endpoint.id === this.endpointID); }
  private get filteredEndpoints() { return this.endpoints.filter((endpoint) => !this.category || endpoint.category === this.category); }

  private selectCategory(value: string) {
    this.category = value;
    const first = this.filteredEndpoints[0];
    if (first && !this.filteredEndpoints.some((endpoint) => endpoint.id === this.endpointID)) this.selectEndpoint(first.id);
  }

  private selectEndpoint(value: string) {
    this.endpointID = value;
    this.responseCode = Object.keys(this.endpoint?.operation.responses ?? {})[0] ?? '200';
    this.copied = this.copyError = '';
    if (DEBUG_OPERATIONS.some((operation) => operation.id === value)) this.resetDebug(value);
  }

  private media(content?: Record<string, MediaType>) { return content?.['application/json'] ?? Object.values(content ?? {})[0]; }
  private bodySchema(endpoint: Endpoint | undefined) { return this.media(endpoint?.operation.requestBody ? this.resolve(endpoint.operation.requestBody).content : undefined)?.schema; }

  private typeName(raw: Schema): string {
    const schema = this.resolve(raw);
    const variants = schema.oneOf ?? schema.anyOf;
    if (variants?.length) return variants.map((variant) => this.typeName(variant)).join(' / ');
    const type = Array.isArray(schema.type) ? schema.type.join(' / ') : schema.type ?? (schema.properties ? 'object' : schema.items ? 'array' : '未指定');
    return type === 'array' ? `array<${schema.items ? this.typeName(schema.items) : 'any'}>` : type + (schema.nullable ? ' / null' : '');
  }

  private constraints(raw: Schema): string {
    const schema = this.resolve(raw);
    return [schema.enum?.length ? '可选值：' + schema.enum.map((value) => typeof value === 'string' ? value : pretty(value)).join('、') : '',
      schema.minimum !== undefined ? `最小值 ${schema.minimum}` : '', schema.maximum !== undefined ? `最大值 ${schema.maximum}` : '',
      schema.minLength !== undefined ? `至少 ${schema.minLength} 个字符` : '', schema.maxLength !== undefined ? `最多 ${schema.maxLength} 个字符` : '', schema.format ? `格式 ${schema.format}` : '', schema.pattern ? `格式规则 ${schema.pattern}` : ''].filter(Boolean).join('；');
  }

  private fields(raw: Schema | undefined, prefix = '', depth = 0, seen = new Set<string>()): FieldRow[] {
    if (!raw || depth > 5 || raw.$ref && seen.has(raw.$ref)) return [];
    const next = new Set(seen); if (raw.$ref) next.add(raw.$ref);
    const schema = this.resolve(raw);
    const fields: FieldRow[] = [];
    for (const [name, child] of Object.entries(schema.properties ?? {})) {
      const path = prefix ? `${prefix}.${name}` : name;
      fields.push({ name: path, schema: child, required: schema.required?.includes(name) ?? false, description: this.resolve(child).description });
      const resolved = this.resolve(child);
      fields.push(...this.fields(resolved.type === 'array' ? resolved.items : child, resolved.type === 'array' ? path + '[]' : path, depth + 1, next));
    }
    for (const nested of schema.allOf ?? []) fields.push(...this.fields(nested, prefix, depth + 1, next));
    return fields;
  }

  private sample(raw: Schema | undefined, field = '', depth = 0, includeOptional = false): unknown {
    if (!raw || depth > 5) return null;
    const schema = this.resolve(raw);
    if (schema.example !== undefined) return schema.example;
    if (schema.default !== undefined) return schema.default;
    if (schema.enum?.length) return field === 'status' && schema.enum.includes(0) ? 0 : schema.enum[0];
    if (schema.oneOf?.length || schema.anyOf?.length) return this.sample((schema.oneOf ?? schema.anyOf)?.[0], field, depth + 1, includeOptional);
    if (schema.properties || schema.type === 'object') {
      const value: Record<string, unknown> = {};
      for (const [name, property] of Object.entries(schema.properties ?? {})) {
        const resolved = this.resolve(property);
        if (includeOptional || schema.required?.includes(name) || resolved.example !== undefined || depth > 0) value[name] = this.sample(property, name, depth + 1, includeOptional);
      }
      return value;
    }
    if (schema.type === 'array') return [this.sample(schema.items, field, depth + 1, includeOptional)];
    if (schema.type === 'boolean') return true;
    if (schema.type === 'integer' || schema.type === 'number') return schema.minimum ?? 0;
    if (field === 'orderNo') return 'PAY_REPLACE_WITH_ORDER_NO';
    if (field === 'requestId') return 'refund_demo_001';
    if (schema.format === 'date-time') return '2026-10-31T00:00:00.000Z';
    if (schema.format === 'uri' || field.toLowerCase().includes('url')) return 'https://example.com/notify';
    return field ? `YOUR_${field.replace(/([a-z])([A-Z])/g, '$1_$2').toUpperCase()}` : 'string';
  }

  private requestExample(endpoint: Endpoint | undefined) {
    const media = this.media(endpoint?.operation.requestBody ? this.resolve(endpoint.operation.requestBody).content : undefined);
    return media?.example ?? Object.values(media?.examples ?? {})[0]?.value ?? this.sample(media?.schema);
  }

  private authLabel(endpoint: Endpoint) {
    const security = endpoint.operation.security ?? this.metadata?.openapi.security ?? [];
    if (!security.length) return '无需鉴权';
    if (endpoint.path.startsWith('/gateway/')) return 'API 令牌';
    if (endpoint.path.startsWith('/admin/') || endpoint.path.startsWith('/auth/')) return '管理员登录凭证';
    return 'Bearer 凭证';
  }

  private curl(endpoint: Endpoint | undefined) {
    if (!endpoint) return '';
    const base = (this.metadata?.baseUrl || location.origin).replace(/\/$/, '');
    let path = endpoint.path;
    for (const parameter of endpoint.parameters.filter((parameter) => parameter.in === 'path')) path = path.replace(`{${parameter.name}}`, encodeURIComponent(String(parameter.example ?? this.sample(parameter.schema, parameter.name))));
    const query = new URLSearchParams();
    for (const parameter of endpoint.parameters.filter((parameter) => parameter.in === 'query' && (parameter.required || parameter.example !== undefined || endpoint.path === '/gateway/query' && parameter.name === 'orderNo'))) query.set(parameter.name, String(parameter.example ?? this.sample(parameter.schema, parameter.name)));
    const parts = ['curl --request ' + endpoint.method + ' ' + shellQuote(base + '/v1/api' + path + (query.size ? '?' + query : ''))];
    const auth = this.authLabel(endpoint);
    if (auth !== '无需鉴权') parts.push('--header ' + shellQuote('Authorization: Bearer ' + (auth === 'API 令牌' ? '<API_TOKEN>' : auth === '管理员登录凭证' ? '<ADMIN_JWT>' : '<BEARER_TOKEN>')));
    if (endpoint.operation.requestBody) { parts.push('--header ' + shellQuote('Content-Type: application/json')); parts.push('--data-raw ' + shellQuote(pretty(this.requestExample(endpoint)))); }
    return parts.join(' \\\n  ');
  }

  private async copy(key: string, value: string) {
    const run = ++this.copyRun;
    this.copyError = '';
    try { await navigator.clipboard.writeText(value); if (this.isConnected && run === this.copyRun) this.copied = key; }
    catch { if (this.isConnected && run === this.copyRun) this.copyError = '复制失败，请选择文本手动复制。'; }
  }

  private resetDebug(value: string) {
    if (!DEBUG_OPERATIONS.some((operation) => operation.id === value)) return;
    this.requests.abort('debug'); this.debugRun++;
    this.debugOperation = value; this.debugBusy = false; this.debugError = ''; this.debugResult = null;
    const endpoint = this.endpoints.find((endpoint) => endpoint.id === value);
    this.queryText = value === 'GET /gateway/query' ? 'orderNo=PAY_REPLACE_WITH_ORDER_NO' : '';
    if (value !== 'GET /gateway/query') this.requestBody = pretty(this.requestExample(endpoint) ?? (value.endsWith('/refund') ? { orderNo: 'PAY_REPLACE_WITH_ORDER_NO', amount: 0.5, requestId: 'refund_demo_001' } : { amount: 1, productName: '联调订单', payMethod: 'native' }));
  }

  private redactResponse(value: string, token: string) { return token ? value.split(token).join('[令牌已隐藏]') : value; }

  private paymentURL(value: unknown, token: string): string | undefined {
    if (!isObject(value) || !isObject(value.data)) return;
    const data = value.data;
    const candidate = data.paymentUrl ?? (data.type === 'url' ? data.data : undefined);
    if (typeof candidate !== 'string' || !candidate || token && candidate.includes(token)) return;
    try { const url = new URL(candidate, location.origin); if (['http:', 'https:'].includes(url.protocol) && !url.username && !url.password) return url.href; } catch { /* Ignore invalid payment URLs. */ }
  }

  private async sendRequest(event: SubmitEvent) {
    event.preventDefault();
    if (this.debugBusy) return;
    this.debugError = validateForm(event.currentTarget as HTMLFormElement) ?? '';
    if (this.debugError) return;
    const operation = DEBUG_OPERATIONS.find((operation) => operation.id === this.debugOperation);
    if (!operation) { this.debugError = '请选择下单、查询订单或退款接口。'; return; }
    const token = this.renderRoot.querySelector<HTMLInputElement>('#debugToken')?.value.trim() ?? '';
    if (!token) { this.debugError = '请填写 API 令牌。'; return; }
    let body: string | undefined;
    if (operation.method !== 'GET') {
      try { const parsed: unknown = JSON.parse(this.requestBody); if (!isObject(parsed)) throw new Error(); body = this.requestBody; }
      catch { this.debugError = '请求体必须是有效的 JSON 对象。'; this.renderRoot.querySelector<HTMLElement>('#requestBody')?.focus(); return; }
    }
    const url = new URL('/v1/api' + operation.path, location.origin);
    const query = new URLSearchParams(this.queryText.trim().replace(/^\?/, ''));
    url.search = query.toString();
    const run = ++this.debugRun;
    const signal = this.requests.next('debug');
    this.debugBusy = true; this.debugResult = null;
    const started = performance.now();
    try {
      const response = await fetch(url, { method: operation.method, headers: { Authorization: 'Bearer ' + token, ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}) }, body, signal, credentials: 'omit', redirect: 'manual', cache: 'no-store' });
      if (response.type === 'opaqueredirect' || response.status >= 300 && response.status < 400) throw new Error('接口返回了跳转，请检查当前服务地址。');
      const text = await response.text();
      signal.throwIfAborted();
      if (!this.isConnected || run !== this.debugRun) return;
      let parsed: unknown;
      try { parsed = JSON.parse(text); } catch { /* Preserve a non-JSON response as text. */ }
      const output = this.redactResponse(parsed === undefined ? text || '响应正文为空' : pretty(parsed), token);
      this.debugResult = { status: response.status, durationMs: performance.now() - started, output: output.slice(0, 65536), truncated: output.length > 65536, paymentURL: this.paymentURL(parsed, token) };
    } catch (error: unknown) {
      if (!isAbort(error) && !signal.aborted && this.isConnected && run === this.debugRun) this.debugError = this.redactResponse(error instanceof Error && !(error instanceof TypeError) ? error.message : '请求失败，请检查网络连接。', token);
    } finally { if (run === this.debugRun) this.debugBusy = false; this.requests.release('debug', signal); }
  }

  private cancelRequest() { this.requests.abort('debug'); this.debugRun++; this.debugBusy = false; this.debugError = '请求已取消。'; }
  private resetVerification() { this.verifyRun++; this.verifyBusy = false; this.verifyError = ''; this.verification = null; }

  private async verifyCallback(event: SubmitEvent) {
    event.preventDefault();
    this.verifyError = validateForm(event.currentTarget as HTMLFormElement) ?? '';
    if (this.verifyError) return;
    try { JSON.parse(this.verifyBody); } catch { this.verifyError = '回调报文必须是有效的 JSON，请保留原始内容。'; this.renderRoot.querySelector<HTMLElement>('#verifyBody')?.focus(); return; }
    if (!['1', '2'].includes(this.verifyVersion)) { this.verifyError = '请选择签名版本。'; return; }
    if (!globalThis.crypto?.subtle) { this.verifyError = '当前浏览器不支持验签，请使用 HTTPS 地址或 localhost。'; return; }
    const secret = this.renderRoot.querySelector<HTMLInputElement>('#signingKey')?.value ?? '';
    const run = ++this.verifyRun;
    this.verifyBusy = true; this.verification = null;
    const encoder = new TextEncoder();
    const payload = encoder.encode(this.verifyBody + '&timestamp=' + this.verifyTimestamp + '&nonce=' + this.verifyNonce);
    const bytes = Uint8Array.from(this.verifySignature.match(/.{2}/g) ?? [], (pair) => parseInt(pair, 16));
    try {
      const key = await crypto.subtle.importKey('raw', encoder.encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign', 'verify']);
      const [valid, signature] = await Promise.all([crypto.subtle.verify('HMAC', key, bytes, payload), crypto.subtle.sign('HMAC', key, payload)]);
      if (this.isConnected && run === this.verifyRun) this.verification = { valid, expected: Array.from(new Uint8Array(signature), (byte) => byte.toString(16).padStart(2, '0')).join('') };
    } catch { if (this.isConnected && run === this.verifyRun) this.verifyError = '验签失败，请检查密钥与报文内容。'; }
    finally { if (run === this.verifyRun) this.verifyBusy = false; }
  }

  private schemaTable(schema: Schema | undefined): TemplateResult {
    const fields = this.fields(schema);
    if (!fields.length) return html`<p class="developer-note">${schema ? `返回类型：${this.typeName(schema)}${this.resolve(schema).description ? '。' + this.resolve(schema).description : ''}` : '此接口未提供字段定义。'}</p>`;
    return html`<div class="table-scroll"><table class="schema-table"><thead><tr><th>字段</th><th>类型</th><th>必填</th><th>说明</th></tr></thead><tbody>
      ${fields.map((field) => html`<tr><td class="mono schema-name">${field.name}</td><td class="mono">${this.typeName(field.schema)}</td><td><span class=${field.required ? 'schema-required' : 'muted'}>${field.required ? '是' : '否'}</span></td><td class="schema-description">${field.description || '—'}${this.constraints(field.schema) ? html`<p class="developer-note">${this.constraints(field.schema)}</p>` : nothing}</td></tr>`)}
    </tbody></table></div>`;
  }

  private renderDocumentation() {
    const endpoint = this.endpoint;
    if (!endpoint) return html`<section class="panel"><p class="developer-note">暂无接口文档。</p></section>`;
    const response = this.resolve(endpoint.operation.responses?.[this.responseCode] ?? {});
    const media = this.media(response.content);
    const schema = this.bodySchema(endpoint);
    return html`<section class="panel" aria-label="接口文档"><div class="developer-filters">
      <div class="field"><label for="apiCategory">接口分类</label><sp-select id="apiCategory" label="接口分类" .value=${this.category} .options=${[{ value: '', label: '全部接口' }, ...[...new Set(this.endpoints.map((endpoint) => endpoint.category))].map((value) => ({ value, label: value }))]} @change=${(event: Event) => this.selectCategory((event.currentTarget as SpSelect).value)}></sp-select></div>
      <div class="field"><label for="apiEndpoint">选择接口</label><sp-select id="apiEndpoint" label="选择接口" .value=${this.endpointID} .options=${this.filteredEndpoints.map((endpoint) => ({ value: endpoint.id, label: `${endpoint.method} ${endpoint.path} · ${endpoint.operation.summary ?? ''}` }))} @change=${(event: Event) => this.selectEndpoint((event.currentTarget as SpSelect).value)}></sp-select></div>
      <button type="button" ?disabled=${this.loading} @click=${() => void this.load()}>刷新文档</button>
    </div><div class="endpoint-heading"><div class="cell-stack"><h2>${endpoint.operation.summary || '接口说明'}</h2><div class="endpoint-path"><span class="method-badge">${endpoint.method}</span><code class="mono">/v1/api${endpoint.path}</code></div></div><span class="badge info">${this.authLabel(endpoint)}</span></div>
    ${endpoint.operation.description ? html`<p class="developer-note endpoint-description">${endpoint.operation.description}</p>` : nothing}
    <div class="docs-grid"><div class="docs-section"><h3>请求参数</h3>
      ${endpoint.parameters.length ? html`<div class="table-scroll"><table class="schema-table"><thead><tr><th>参数</th><th>位置</th><th>类型 / 必填</th><th>说明</th></tr></thead><tbody>${endpoint.parameters.map((parameter) => html`<tr><td class="mono">${parameter.name}</td><td>${({ query: '查询参数', path: '路径参数', header: '请求头', cookie: 'Cookie' } as Record<string, string>)[parameter.in] || parameter.in}</td><td>${this.typeName(parameter.schema ?? {})}${parameter.required ? html`<p class="schema-required">必填</p>` : nothing}</td><td>${parameter.description || this.resolve(parameter.schema ?? {}).description || '—'}${this.constraints(parameter.schema ?? {}) ? html`<p class="developer-note">${this.constraints(parameter.schema ?? {})}</p>` : nothing}</td></tr>`)}</tbody></table></div>` : html`<p class="developer-note">无路径或查询参数。</p>`}
      ${endpoint.operation.requestBody ? html`<h3>JSON 请求体</h3>${this.schemaTable(schema)}<pre class="code-box mono" data-request-example>${pretty(this.requestExample(endpoint))}</pre>` : nothing}
    </div><div class="docs-section"><div class="field"><label for="responseCode">响应状态</label><sp-select id="responseCode" label="响应状态" .value=${this.responseCode} .options=${Object.entries(endpoint.operation.responses ?? {}).map(([value, response]) => ({ value, label: `${value} · ${this.resolve(response).description || '响应'}` }))} @change=${(event: Event) => { this.responseCode = (event.currentTarget as SpSelect).value; }}></sp-select></div>
      <p class="developer-note">${response.description || '暂无状态说明'}</p>${this.schemaTable(media?.schema)}${media?.schema ? html`<pre class="code-box mono" data-response-example>${pretty(media.example ?? this.sample(media.schema, '', 0, true))}</pre>` : nothing}
    </div></div><div class="docs-section"><div class="code-header"><h3>curl 请求示例</h3><button type="button" data-copy-curl @click=${() => void this.copy('curl', this.curl(endpoint))}>${this.copied === 'curl' ? '已复制' : '复制 curl'}</button></div><pre class="code-box mono" data-curl>${this.curl(endpoint)}</pre><p class="developer-note">将示例中的凭证占位符替换为你的令牌，路径中的示例值替换为实际值。</p></div></section>`;
  }

  private renderDebugger() {
    const operation = DEBUG_OPERATIONS.find((operation) => operation.id === this.debugOperation)!;
    return html`<section class="panel"><div class="panel-head"><h2>在线请求调试</h2><span class="badge info">API 令牌</span></div><p class="developer-note">调试下单、订单查询和退款。请求会在当前环境执行，支付链接需手动打开。同一笔订单复用 externalOrderNo，重试退款复用 requestId。</p>
      <form class="developer-form" novalidate @submit=${this.sendRequest}><div class="field"><label for="debugEndpoint">调试接口</label><sp-select id="debugEndpoint" label="调试接口" .value=${this.debugOperation} .options=${DEBUG_OPERATIONS.map((operation) => ({ value: operation.id, label: operation.label }))} @change=${(event: Event) => this.resetDebug((event.currentTarget as SpSelect).value)}></sp-select></div>
      <div class="field"><label for="debugToken">API 令牌</label><input id="debugToken" type="password" required autocomplete="off" spellcheck="false" placeholder="输入当前项目的 API 令牌"></div>
      <div class="field"><label for="queryText">查询参数</label><textarea id="queryText" class="query-input mono" maxlength="10000" placeholder=${operation.method === 'GET' ? 'externalOrderNo=demo_order_001' : 'key=value'} .value=${this.queryText} @input=${(event: Event) => { this.queryText = (event.currentTarget as HTMLTextAreaElement).value; this.debugError = ''; }}></textarea><p class="developer-note">${operation.method === 'GET' ? '查单时填写 orderNo 或 externalOrderNo，且只能选择一项。' : '按 key=value 填写，多项以 & 分隔。'}</p></div>
      ${operation.method !== 'GET' ? html`<div class="field"><label for="requestBody">JSON 请求体</label><textarea id="requestBody" required maxlength="65536" spellcheck="false" .value=${this.requestBody} @input=${(event: Event) => { this.requestBody = (event.currentTarget as HTMLTextAreaElement).value; this.debugError = ''; }}></textarea></div>` : nothing}
      <div class="form-actions"><button class="primary" type="submit" data-send-request ?disabled=${this.debugBusy}>${this.debugBusy ? '请求中…' : '发送请求'}</button>${this.debugBusy ? html`<button type="button" @click=${this.cancelRequest}>取消请求</button>` : nothing}<span class="developer-note mono">${operation.method} /v1/api${operation.path}</span></div>
      ${this.debugError ? html`<p class="err-text small" role="alert" data-debug-error>${this.debugError}</p>` : nothing}</form>
      ${this.debugResult ? html`<div class="developer-response" aria-live="polite"><div class="response-meta"><span class=${`badge ${this.debugResult.status >= 200 && this.debugResult.status < 300 ? 'ok' : 'err'}`}>HTTP ${this.debugResult.status}</span><span class="mono">${this.debugResult.durationMs.toFixed(1)} ms</span><button type="button" class="link" @click=${() => void this.copy('response', this.debugResult!.output)}>${this.copied === 'response' ? '已复制' : '复制响应'}</button></div><pre class="code-box mono" data-debug-output>${this.debugResult.output}</pre>${this.debugResult.truncated ? html`<p class="developer-note">响应较长，仅显示前 64 KiB。</p>` : nothing}${this.debugResult.paymentURL ? html`<a class="download-link" href=${this.debugResult.paymentURL} target="_blank" rel="noopener noreferrer" data-payment-link>打开支付链接 ↗</a>` : nothing}</div>` : nothing}</section>`;
  }

  private renderVerifier() {
    return html`<section class="panel"><h2>回调验签</h2><p class="developer-note">粘贴接收到的原始报文与签名信息。密钥只用于浏览器本地计算。</p>
      <form class="developer-form" novalidate @submit=${this.verifyCallback}>
        <div class="form-pair"><div class="field"><label for="signingKey">回调签名密钥</label><input id="signingKey" type="password" required autocomplete="off" spellcheck="false" placeholder="输入项目的回调签名密钥" @input=${this.resetVerification}></div>
          <div class="field"><label for="verifyVersion">签名版本</label><sp-select id="verifyVersion" label="签名版本" required .value=${this.verifyVersion} .options=${[{ value: '1', label: '版本 1' }, { value: '2', label: '版本 2' }]} @change=${(event: Event) => { this.verifyVersion = (event.currentTarget as SpSelect).value; this.resetVerification(); }}></sp-select></div></div>
        <div class="field"><label for="verifyBody">原始 JSON 报文</label><textarea id="verifyBody" required maxlength="65536" spellcheck="false" placeholder='{"orderNo":"PAY…","status":"paid"}' .value=${this.verifyBody} @input=${(event: Event) => { this.verifyBody = (event.currentTarget as HTMLTextAreaElement).value; this.resetVerification(); }}></textarea><p class="developer-note">保留原始空格、字段顺序和换行，修改报文会改变签名。</p></div>
        <div class="form-pair"><div class="field"><label for="verifyTimestamp">Timestamp 时间戳</label><input id="verifyTimestamp" required pattern="[0-9]+" maxlength="20" inputmode="numeric" spellcheck="false" .value=${this.verifyTimestamp} @input=${(event: Event) => { this.verifyTimestamp = (event.currentTarget as HTMLInputElement).value; this.resetVerification(); }}></div>
          <div class="field"><label for="verifyNonce">Nonce 随机串</label><input id="verifyNonce" required maxlength="128" spellcheck="false" .value=${this.verifyNonce} @input=${(event: Event) => { this.verifyNonce = (event.currentTarget as HTMLInputElement).value; this.resetVerification(); }}></div></div>
        <div class="field"><label for="verifySignature">Signature 签名</label><input id="verifySignature" required pattern="[a-fA-F0-9]{64}" maxlength="64" spellcheck="false" placeholder="64 位十六进制签名" .value=${this.verifySignature} @input=${(event: Event) => { this.verifySignature = (event.currentTarget as HTMLInputElement).value; this.resetVerification(); }}></div>
        <p class="developer-note">算法：HMAC-SHA256。签名内容为原始报文 + &amp;timestamp=时间戳 + &amp;nonce=随机串，版本 1 与版本 2 使用相同算法。</p>
        <div class="form-actions"><button class="primary" type="submit" data-verify ?disabled=${this.verifyBusy}>${this.verifyBusy ? '验签中…' : '验证签名'}</button></div>${this.verifyError ? html`<p class="err-text small" role="alert" data-verify-error>${this.verifyError}</p>` : nothing}
      </form>${this.verification ? html`<div class="verify-result" data-valid=${String(this.verification.valid)} role="status"><strong>${this.verification.valid ? '验签通过' : '签名不匹配'}</strong><p class="developer-note">${this.verification.valid ? '签名与当前密钥和原始报文一致。' : '请检查密钥、报文原始字节与签名头。'}</p><div class="code-header"><span class="developer-note">本地计算签名</span><button type="button" @click=${() => void this.copy('signature', this.verification!.expected)}>${this.copied === 'signature' ? '已复制' : '复制签名'}</button></div><code class="mono">${this.verification.expected}</code></div>` : nothing}</section>`;
  }

  protected override render() {
    if (!this.authorized) return nothing;
    return html`<main class="wrap"><div class="page-head"><h1>开发者接入</h1><p>查阅接口、调试请求并验证支付回调。</p></div>
      ${this.metadata ? html`<section class="panel developer-intro"><div class="cell-stack"><div class="response-meta"><strong>${this.metadata.openapi.info?.title || 'Mintweft API'}</strong><span class=${`badge ${this.metadata.sandboxEnabled ? 'info' : 'warn'}`}>${this.metadata.sandboxEnabled ? '沙箱环境' : '当前环境'}</span><span class="developer-note">${this.endpoints.length} 个接口</span></div><span class="api-base mono">${this.metadata.baseUrl}/v1/api</span></div><div class="developer-actions"><a class="download-link" href="/v1/api/openapi.json" download="mintweft-openapi.json">下载 OpenAPI</a><a class="download-link" href="/v1/api/developer/example" download="mintweft-demo.mjs">下载 Node.js 示例</a></div></section>` : nothing}
      ${this.loadError ? html`<section class="panel"><p class="err-text" role="alert">接口文档加载失败：${this.loadError}</p><div><button type="button" @click=${() => void this.load()}>重试加载</button></div></section>` : nothing}
      ${!this.metadata && this.loading ? html`<section class="panel loading-panel" role="status">正在加载接口文档…</section>` : nothing}
      ${this.metadata ? html`${this.renderDocumentation()}<div class="developer-tools">${this.renderDebugger()}${this.renderVerifier()}</div>` : nothing}
      ${this.copyError ? html`<p class="err-text" role="status">${this.copyError}</p>` : nothing}
    </main>`;
  }
}

customElements.define('sp-developer-page', SpDeveloperPage);
