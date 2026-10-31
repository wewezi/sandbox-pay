import { css } from 'lit';
export const operationsStyles = css`
  .tabs { display:flex; flex-wrap:wrap; gap:var(--s2); border-bottom:1px solid var(--line); padding-bottom:var(--s3); }
  .tabs .link { margin-left:auto; }
  .tabs button[aria-selected="true"] { color:var(--brand); border-color:var(--brand); background:var(--brand-soft); }
  .overview-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:var(--s4); align-items:start; }
  .channel-list { display:flex; flex-direction:column; gap:var(--s3); }
  .channel { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--s3); padding:var(--s4); border:1px solid var(--line); border-radius:var(--r-sm); background:var(--panel-2); }
  .channel > div { min-width:0; }
  .channel strong { font-size:13px; font-weight:600; }
  .channel .badge { flex:none; }
  .channel p { margin-top:var(--s1); font-size:12px; color:var(--muted); line-height:1.7; overflow-wrap:anywhere; }
  .settings-grid { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:var(--s4); }
  .note { font-size:12px; color:var(--muted); line-height:1.7; }
  .ok-text { color:var(--ok); }
  td .mono { overflow-wrap:anywhere; }
  .audit-filter { display:flex; align-items:flex-end; gap:var(--s2); }
  .audit-filter .field { flex:1; max-width:480px; }
  .backup-name { min-width:240px; max-width:400px; white-space:normal; overflow-wrap:anywhere; line-height:1.65; }
  .audit-table td:nth-child(4) { min-width:220px; max-width:360px; white-space:normal; overflow-wrap:anywhere; line-height:1.65; }
  .audit-table td:nth-child(7) { color:var(--muted); }
  .settings-grid input { font-variant-numeric:tabular-nums; }
  @media(max-width:1000px) { .overview-grid { grid-template-columns:1fr; } .settings-grid { grid-template-columns:repeat(2,minmax(0,1fr)); } }
  @media(max-width:720px) {
    .tabs { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); }
    .tabs .link { grid-column:1/-1; justify-self:end; margin-left:0; padding:4px var(--s3); }
    .stats { grid-template-columns:repeat(2,minmax(0,1fr)); gap:var(--s3); }
    .stats .stat:last-child { grid-column:1/-1; }
    .channel { padding:var(--s3); }
  }
  @media(max-width:600px) { .settings-grid { grid-template-columns:1fr; } .audit-filter { flex-wrap:wrap; } .audit-filter .field { flex-basis:100%; } }
`;
