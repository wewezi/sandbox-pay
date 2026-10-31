import { css } from 'lit';

export const usageStyles = css`
.usage-tabs { display: flex; gap: var(--s2); border-bottom: 1px solid var(--line); }
.usage-tabs button { padding: 10px var(--s5); border: 0; border-radius: var(--r-sm) var(--r-sm) 0 0; background: none; color: var(--muted); }
.usage-tabs button[aria-selected="true"] { color: var(--brand); background: var(--info-bg); box-shadow: inset 0 -2px var(--brand); }
.usage-content { display: flex; flex-direction: column; gap: var(--s5); min-width: 0; }
.usage-stats { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.stat .ok-text { color: var(--ok); }
.stat .error-value { color: var(--err); }
.usage-filter-panel { padding: var(--s4); }
.filter-grid { display: grid; grid-template-columns: minmax(140px, 1.7fr) minmax(100px, 1fr) minmax(104px, 1fr) minmax(100px, .75fr) minmax(100px, .75fr) 134px 134px auto; align-items: end; gap: var(--s2); }
.filter-grid sp-select, .filter-grid sp-date-picker { --control-padding-inline: var(--s2); }
.filter-actions { display: flex; align-items: center; gap: var(--s2); }
.filter-note { color: var(--muted); font-size: 12px; }
.usage-table { min-width: 1000px; }
.usage-row { cursor: pointer; }
.usage-row:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
.cell-stack { display: flex; flex-direction: column; gap: 4px; }
.cell-stack .secondary { color: var(--muted); font-size: 11.5px; }
.endpoint-cell { min-width: 180px; max-width: 340px; }
.endpoint-cell .mono { white-space: normal; overflow-wrap: anywhere; }
.endpoint-method { font-size: 11px; font-weight: 650; color: var(--muted); }
.source-cell { min-width: 180px; max-width: 280px; }
.source-cell .secondary { white-space: normal; overflow-wrap: anywhere; }
.identity-cell { min-width: 140px; max-width: 230px; }
.identity-cell span { white-space: normal; overflow-wrap: anywhere; }
.result-code { margin-top: 2px; color: var(--muted); font-size: 11px; }
dialog.usage-dialog { --usage-dialog-padding: var(--s6); max-width: 740px !important; max-height: calc(100dvh - var(--s8)); padding: var(--usage-dialog-padding); overflow: auto; }
dialog.usage-dialog[open] { gap: var(--s5); }
.usage-dialog > * { flex-shrink: 0; }
.usage-dialog-header { position: sticky; top: calc(0px - var(--usage-dialog-padding)); z-index: 2; display: flex; align-items: flex-start; justify-content: space-between; gap: var(--s4); margin: calc(0px - var(--usage-dialog-padding)) calc(0px - var(--usage-dialog-padding)) 0; padding: var(--usage-dialog-padding); background: var(--panel); border-bottom: 1px solid var(--line); }
.usage-dialog-header h3 { font-size: 17px; color: var(--text); }
.usage-dialog-header .cell-stack { min-width: 0; }
.usage-dialog-header .mono { overflow-wrap: anywhere; }
.usage-dialog-header button { flex: none; }
.usage-detail-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); margin: 0; gap: var(--s5) var(--s6); }
.usage-detail-fields > div { min-width: 0; }
.usage-detail-fields .full { grid-column: 1 / -1; }
.usage-detail-fields dt { color: var(--muted); font-size: 12px; margin-bottom: 6px; }
.usage-detail-fields dd { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--s2); margin: 0; font-size: 13px; overflow-wrap: anywhere; }
.usage-detail-fields dd > span { min-width: 0; }
.usage-detail-fields .missing { color: var(--muted); }
.usage-detail-fields button { font-size: 11.5px; padding: 2px var(--s2); }
.usage-error { padding: var(--s4); border-radius: var(--r-sm); background: var(--err-bg); color: var(--err); font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
.usage-dialog-footer { position: sticky; bottom: calc(0px - var(--usage-dialog-padding)); z-index: 2; display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: var(--s3); margin: 0 calc(0px - var(--usage-dialog-padding)) calc(0px - var(--usage-dialog-padding)); border-top: 1px solid var(--line); padding: var(--s4) var(--usage-dialog-padding); background: var(--panel); }
.callback-body, .callback-history, .callback-simulation { margin: 0; }
.callback-body h4, .callback-history h4, .callback-simulation h4 { color: var(--text); font-size: 14px; margin: 0 0 var(--s3); }
.callback-body .panel-head { margin-bottom: var(--s2); }
.callback-body .panel-head h4 { margin: 0; }
.callback-body pre, .callback-response pre { white-space: pre-wrap; overflow-wrap: anywhere; margin: var(--s2) 0 0; font-size: 12px; line-height: 1.65; max-height: 280px; overflow: auto; }
.callback-body pre { padding: var(--s4); background: var(--bg); border: 1px solid var(--line); border-radius: var(--r-sm); }
.callback-history ol { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: var(--s3); }
.callback-attempt { padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r-sm); }
.callback-attempt-head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); margin-bottom: var(--s2); font-size: 12px; }
.callback-attempt p { margin-top: var(--s2); font-size: 12px; overflow-wrap: anywhere; }
.callback-response { margin-top: var(--s3); }
.callback-simulation { border-top: 1px solid var(--line); padding-top: var(--s4); }
.callback-simulation form { display: flex; align-items: end; flex-wrap: wrap; gap: var(--s3); margin-top: var(--s3); }
.callback-simulation .field { width: 130px; }
.simulation-result { font-size: 12px; margin-top: var(--s3); color: var(--muted); }
@media (max-width: 1399px) {
  .filter-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .filter-actions { grid-column: span 2; }
}
@media (max-width: 900px) {
  .filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .filter-grid .keyword-field { grid-column: 1 / -1; }
  .filter-actions { grid-column: 1 / -1; flex-wrap: wrap; }
}
@media (max-width: 720px) {
  .usage-content { gap: var(--s4); }
  .usage-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s3); }
  .usage-stats .stat { padding: var(--s4); }
  .usage-stats .v { font-size: 23px; }
  dialog.usage-dialog { --usage-dialog-padding: var(--s4); width: calc(100% - var(--s6)) !important; max-height: calc(100dvh - var(--s6)); }
  .usage-detail-fields { gap: var(--s4); }
}
@media (max-width: 380px) {
  .usage-detail-fields { grid-template-columns: 1fr; }
}
`;
