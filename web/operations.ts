import { LitElement, html, nothing } from 'lit';
import { api, confirmDialog, count, dateTime, isAbort, ok, requireAuth } from './app';
import { RequestController } from './controllers/requests';
import { downloadAPI } from './services/download';
import { validateForm } from './services/form';
import { pageStyles, sharedStyles } from './styles/shared';
import { operationsStyles } from './styles/operations';

type Settings = { usageRetentionDays:number; auditRetentionDays:number; attemptRetentionDays:number; backupIntervalHours:number; backupKeepCount:number; updatedAt?:string };
type Channel = { id:string; name:string; configured:boolean; reachable?:boolean; message:string };
type Run = { name:string; lastRunAt:string; lastError:string };
type Status = { database:string; schemaVersion:number; counts:Record<string,number>; channels:Channel[]; runs:Run[]; settings:Settings };
type Backup = { name:string; size:number; createdAt:string; configIncluded:boolean };
type Audit = { id:number; username:string; method:string; path:string; action:string; target:string; clientIp:string; statusCode:number; durationMs:number; requestId:string; createdAt:string };
type AuditPage = { items:Audit[]; total:number; page:number; pageSize:number };
type Tab = 'overview'|'backups'|'audit'|'settings';
type State = { authorized:boolean; superAdmin:boolean; loading:boolean; busy:string; error:string; tab:Tab; status:Status|null; backups:Backup[]; audit:AuditPage; keyword:string; appliedKeyword:string; settings:Settings };
const defaults = ():Settings=>({usageRetentionDays:90,auditRetentionDays:180,attemptRetentionDays:90,backupIntervalHours:24,backupKeepCount:7});
const settingFields:readonly [keyof Settings,string,number,number][] = [['usageRetentionDays','调用日志保留天数',1,3650],['auditRetentionDays','操作审计保留天数',1,3650],['attemptRetentionDays','回调历史保留天数',1,3650],['backupIntervalHours','自动备份间隔（小时）',0,720],['backupKeepCount','备份保留份数',1,100]];

class SpOperationsPage extends LitElement {
  static override properties = { state:{state:true} };
  static override styles = [sharedStyles,pageStyles,operationsStyles];
  declare private state:State;
  private readonly requests = new RequestController(this,()=>void this.load());
  constructor(){super();this.state={authorized:false,superAdmin:false,loading:false,busy:'',error:'',tab:'overview',status:null,backups:[],audit:{items:[],total:0,page:1,pageSize:50},keyword:'',appliedKeyword:'',settings:defaults()};}
  private set(values:Partial<State>){this.state={...this.state,...values};}
  override connectedCallback(){super.connectedCallback();this.set({authorized:requireAuth()});if(this.state.authorized)void this.load();}
  override disconnectedCallback(){this.set({loading:false,busy:''});super.disconnectedCallback();}
  private async load(){
    if(!this.isConnected||!this.state.authorized)return;
    const signal=this.requests.next('load');this.set({loading:true,error:''});
    try{
      const account=await api.get<{role:string}>('/auth/session',{signal});
      if(signal.aborted||!this.isConnected)return;
      if(account.role!=='SuperAdmin'){this.set({superAdmin:false,error:'运行维护仅对超级管理员开放。'});return;}
      const [status,backups]=await Promise.all([api.get<Status>('/admin/operations/status',{signal}),api.get<Backup[]>('/admin/operations/backups',{signal})]);
      if(signal.aborted||!this.isConnected)return;this.set({superAdmin:true,status,backups,settings:{...status.settings}});
      if(this.state.tab==='audit')await this.loadAudit(1);
    }catch(error){if(!isAbort(error)&&!signal.aborted&&this.isConnected)this.set({error:error instanceof Error?error.message:String(error)});}
    finally{if(!signal.aborted&&this.isConnected)this.set({loading:false});this.requests.release('load',signal);}
  }
  private async loadAudit(page:number){
    if(!this.isConnected||!this.state.authorized)return;
    const signal=this.requests.next('audit');this.set({error:''});
    try{const query=new URLSearchParams({page:String(page),pageSize:'50',keyword:this.state.appliedKeyword});const audit=await api.get<AuditPage>('/admin/audit-logs?'+query,{signal});if(!signal.aborted&&this.isConnected)this.set({audit});}
    catch(error){if(!isAbort(error)&&!signal.aborted&&this.isConnected)this.set({error:error instanceof Error?error.message:String(error)});}
    finally{this.requests.release('audit',signal);}
  }
  private selectTab(tab:Tab){this.set({tab,error:''});if(tab==='audit')void this.loadAudit(this.state.audit.page);}
  private async run(action:string,work:(signal:AbortSignal)=>Promise<void>){
    if(!this.isConnected||this.state.busy)return;
    const signal=this.requests.next('action');
    this.set({busy:action,error:''});
    try{await work(signal);}
    catch(error){if(!isAbort(error)&&!signal.aborted&&this.isConnected)this.set({error:error instanceof Error?error.message:String(error)});}
    finally{if(!signal.aborted&&this.isConnected)this.set({busy:''});this.requests.release('action',signal);}
  }
  private backup(){void this.run('backup',async signal=>{
    await api.post('/admin/operations/backups',{}, {signal});
    const backups=await api.get<Backup[]>('/admin/operations/backups',{signal});
    if(signal.aborted||!this.isConnected)return;
    this.set({backups});ok('备份已创建');
  });}
  private checkChannels(){void this.run('channels',async signal=>{
    const channels=await api.post<Channel[]>('/admin/operations/channels/check',{}, {signal});
    if(!signal.aborted&&this.isConnected&&this.state.status)this.set({status:{...this.state.status,channels}});
  });}
  private saveSettings(event:SubmitEvent){
    event.preventDefault();const error=validateForm(event.currentTarget as HTMLFormElement);if(error){this.set({error});return;}
    const body=Object.fromEntries(settingFields.map(([field])=>[field,this.state.settings[field]]));
    void this.run('settings',async signal=>{
      const settings=await api.post<Settings>('/admin/operations/settings',body,{signal});
      if(signal.aborted||!this.isConnected)return;
      this.set({settings,status:this.state.status?{...this.state.status,settings}:null});ok('运行设置已保存');
    });
  }
  private cleanup(){void this.run('cleanup',async signal=>{
    const confirmed=await confirmDialog({title:'清理过期日志',message:'按当前保留天数删除过期的调用日志、操作审计和回调投递历史。删除后无法恢复。',confirmText:'清理过期日志',danger:true});
    if(!confirmed||signal.aborted||!this.isConnected)return;
    const result=await api.post<Record<string,number>>('/admin/operations/cleanup',{}, {signal});
    if(signal.aborted||!this.isConnected)return;
    ok(`已清理 ${Object.values(result).reduce((sum,n)=>sum+n,0)} 条记录`);await this.load();
  });}
  private exportData(kind:'orders'|'usage'){void this.run('export',signal=>downloadAPI(`/admin/operations/exports/${kind}.csv`,`sandbox-${kind}.csv`,{signal}));}
  private renderOverview(){
    const status=this.state.status;if(!status)return nothing;
    const cards:[string,number][]=[['订单记录',status.counts.payment_orders||0],['调用日志',status.counts.api_usage_logs||0],['待发回调',status.counts.pendingCallbacks||0],['失败回调',status.counts.failedCallbacks||0],['退款处理中',status.counts.pendingRefunds||0]];
    const taskNames:Record<string,string>={backup:'数据库备份',cleanup:'日志清理',notifications:'回调投递',sandbox:'场景任务',expiration:'订单过期处理'};
    return html`<div class="stats">${cards.map(([name,value])=>html`<div class="stat"><span class="k">${name}</span><span class="v">${count(value)}</span></div>`)}</div>
      <div class="overview-grid"><section class="panel"><div class="panel-head"><h2>支付渠道</h2><button ?disabled=${!!this.state.busy} @click=${this.checkChannels}>${this.state.busy==='channels'?'检查中…':'检查连接'}</button></div>
      <div class="channel-list">${status.channels.map(channel=>html`<div class="channel"><div><strong>${channel.name}</strong><p>${channel.message}</p></div><span class=${`badge ${channel.reachable===false?'err':channel.configured?'ok':''}`}>${channel.reachable===false?'连接失败':channel.configured?'已配置':'未配置'}</span></div>`)}</div><p class="note">网络检查不会发起支付。渠道凭证通过运行配置管理。</p></section>
      <section class="panel"><h2>后台任务</h2><p class="small">数据库 <span class="badge ok">正常</span></p><div class="table-scroll"><table><thead><tr><th>任务</th><th>最近运行</th><th>结果</th></tr></thead><tbody>${status.runs.map(run=>html`<tr><td>${taskNames[run.name]||run.name}</td><td>${dateTime(run.lastRunAt)}</td><td>${run.lastError?html`<span class="err-text">${run.lastError}</span>`:html`<span class="ok-text">完成</span>`}</td></tr>`)}</tbody></table></div>${!status.runs.length?html`<p class="muted">暂无运行记录</p>`:nothing}</section></div>
      <section class="panel"><h2>数据导出</h2><div class="row"><button ?disabled=${!!this.state.busy} @click=${()=>this.exportData('orders')}>导出订单 CSV</button><button ?disabled=${!!this.state.busy} @click=${()=>this.exportData('usage')}>导出调用日志 CSV</button></div></section>`;
  }
  private renderBackups(){return html`<section class="panel"><div class="panel-head"><h2>数据库备份</h2><button class="primary" ?disabled=${!!this.state.busy} @click=${this.backup}>${this.state.busy==='backup'?'备份中…':'立即备份'}</button></div>
    <p class="note">备份保存当时的完整数据库。自动备份间隔和保留份数可在运行设置中调整。</p><div class="table-scroll"><table><thead><tr><th>文件</th><th>创建时间</th><th>大小</th><th>配置副本</th><th></th></tr></thead><tbody>${this.state.backups.map(item=>html`<tr><td class="backup-name mono">${item.name}</td><td>${dateTime(item.createdAt)}</td><td>${(item.size/1024).toFixed(1)} KB</td><td>${item.configIncluded?'已保存':'无配置文件'}</td><td><button class="link" ?disabled=${!!this.state.busy} @click=${()=>void this.run('download',signal=>downloadAPI('/admin/operations/backups/'+encodeURIComponent(item.name),item.name,{signal}))}>下载数据库</button></td></tr>`)}</tbody></table></div>${!this.state.backups.length?html`<p class="muted">暂无备份</p>`:nothing}<p class="note">恢复前停止服务并保留当前数据副本；配置副本保存在服务端备份目录。</p></section>`;}
  private renderAudit(){return html`<section class="panel"><h2>管理操作审计</h2><form class="audit-filter" novalidate @submit=${(event:SubmitEvent)=>{event.preventDefault();this.set({appliedKeyword:this.state.keyword.trim()});void this.loadAudit(1);}}><div class="field"><label for="auditKeyword">关键词</label><input id="auditKeyword" placeholder="账号 / 操作 / 资源 / IP / 请求ID" .value=${this.state.keyword} @input=${(event:Event)=>this.set({keyword:(event.currentTarget as HTMLInputElement).value})}></div><button class="primary" type="submit">查询</button><button type="button" @click=${()=>{this.set({keyword:'',appliedKeyword:''});void this.loadAudit(1);}}>清除筛选</button></form>
    <div class="table-scroll"><table class="audit-table"><thead><tr><th>操作时间</th><th>账号</th><th>操作</th><th>资源</th><th>来源 IP</th><th>结果</th><th>请求 ID</th></tr></thead><tbody>${this.state.audit.items.map(item=>html`<tr><td>${dateTime(item.createdAt)}</td><td>${item.username}</td><td>${item.action}</td><td><span class="mono">${item.target||'—'}</span></td><td class="mono">${item.clientIp||'—'}</td><td><span class=${`badge ${item.statusCode<400?'ok':'err'}`}>HTTP ${item.statusCode}</span></td><td class="mono">${item.requestId||'—'}</td></tr>`)}</tbody></table></div>${!this.state.audit.items.length?html`<p class="muted">暂无符合条件的操作记录</p>`:nothing}<sp-pager .page=${this.state.audit.page} .pageSize=${this.state.audit.pageSize} .total=${this.state.audit.total} @page-change=${(event:CustomEvent<{page:number}>)=>void this.loadAudit(event.detail.page)}></sp-pager></section>`;}
  private renderSettings(){return html`<section class="panel"><h2>运行设置</h2><form class="stack" novalidate @submit=${this.saveSettings}><div class="settings-grid">${settingFields.map(([field,label,min,max])=>html`<div class="field"><label for=${field}>${label}</label><input id=${field} type="number" min=${min} max=${max} step="1" required .value=${String(this.state.settings[field])} @input=${(event:Event)=>this.set({settings:{...this.state.settings,[field]:Number((event.currentTarget as HTMLInputElement).value)}})}></div>`)}</div><p class="note">自动备份间隔设为 0 可关闭自动备份。日志按保留天数定期清理。</p><div class="row"><button class="primary" type="submit" ?disabled=${!!this.state.busy}>保存设置</button><button type="button" ?disabled=${!!this.state.busy} @click=${this.cleanup}>立即清理过期日志</button></div></form></section>`;}
  protected override render(){return html`<main class="wrap"><header class="page-head"><h1>运行维护</h1><p>查看运行状态、管理备份和日志，追踪后台操作。</p></header>${this.state.error?html`<p class="err-text" role="alert">${this.state.error}</p>`:nothing}${this.state.loading?html`<p class="muted" role="status">正在加载…</p>`:nothing}${this.state.superAdmin?html`<div class="tabs" role="tablist" aria-label="运行维护功能">${([['overview','运行总览'],['backups','数据备份'],['audit','操作审计'],['settings','运行设置']] as const).map(([tab,label])=>html`<button role="tab" aria-selected=${this.state.tab===tab} @click=${()=>this.selectTab(tab)}>${label}</button>`)}<button class="link" @click=${()=>void this.load()}>刷新</button></div>${this.state.tab==='overview'?this.renderOverview():this.state.tab==='backups'?this.renderBackups():this.state.tab==='audit'?this.renderAudit():this.renderSettings()}`:nothing}</main>`;}
}
customElements.define('sp-operations-page',SpOperationsPage);
