import { css } from 'lit';

export const developerStyles = css`
.developer-intro { display: flex; flex-direction: row; flex-wrap: wrap; justify-content: space-between; gap: var(--s4); align-items: center; }
.developer-intro .cell-stack { min-width: 0; }
.cell-stack { display: flex; flex-direction: column; gap: var(--s2); }
.developer-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); }
.download-link { display: inline-flex; align-items: center; justify-content: center; min-height: 38px; padding: 8px var(--s4); border: 1px solid var(--line); border-radius: var(--r-sm); color: var(--text); font-size: 13px; line-height: 20px; }
.download-link:hover { background: var(--line-soft); }
.api-base { overflow-wrap: anywhere; font-size: 13px; }
.developer-filters { display: grid; grid-template-columns: minmax(150px, .65fr) minmax(260px, 2fr) auto; gap: var(--s3); align-items: end; padding-bottom: var(--s5); border-bottom: 1px solid var(--line-soft); }
.endpoint-heading { display: flex; align-items: flex-start; justify-content: space-between; flex-wrap: wrap; gap: var(--s3); }
.endpoint-heading h2 { font-size: 17px; }
.endpoint-path { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); }
.endpoint-path code { overflow-wrap: anywhere; font-size: 13px; }
.method-badge { padding: 3px 8px; border-radius: var(--r-sm); background: var(--info-bg); color: var(--brand); font-weight: 650; font-size: 11px; }
.docs-grid, .developer-tools { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s5); min-width: 0; }
.developer-tools { align-items: start; }
.docs-section { display: flex; flex-direction: column; gap: var(--s4); min-width: 0; }
.docs-section h3, .developer-tools h3 { color: var(--text); font-size: 13px; }
.schema-table { font-size: 12px; min-width: 430px; }
.schema-table th { white-space: nowrap; }
.schema-table th, .schema-table td { padding: 10px var(--s3); }
.schema-table td { vertical-align: top; white-space: normal; overflow-wrap: anywhere; line-height: 1.7; }
.schema-table td:first-child { white-space: nowrap; }
.schema-table .schema-name { min-width: 140px; }
.schema-table .schema-description { min-width: 140px; }
.schema-table td:nth-child(2) { min-width: 80px; white-space: nowrap; }
.docs-section .table-scroll { margin: 0; padding: 0; width: 100%; border: 1px solid var(--line); border-radius: var(--r-sm); }
.schema-required { color: var(--brand); font-size: 11px; font-weight: 600; }
.panel[aria-label="接口文档"] > .docs-section { padding-top: var(--s4); border-top: 1px solid var(--line-soft); }
.code-box { border: 1px solid var(--line); background: var(--bg); border-radius: var(--r-sm); padding: var(--s4); margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; overflow: auto; max-height: 360px; font-size: 12px; line-height: 1.65; }
.code-header { display: flex; justify-content: space-between; align-items: center; gap: var(--s3); }
.code-header button { min-height: 30px; padding: 4px var(--s3); font-size: 12px; }
.developer-note { color: var(--muted); font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; }
.developer-form { display: flex; flex-direction: column; gap: var(--s4); }
.developer-form .form-pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s3); }
.developer-form textarea { width: 100%; min-height: 144px; resize: vertical; padding: 8px var(--s3); font-family: var(--mono); color: var(--text); background: var(--panel); border: 1px solid var(--line); border-radius: var(--r-sm); line-height: 1.65; font-size: 12px; -webkit-tap-highlight-color: transparent; }
.developer-form textarea::placeholder { color: var(--muted); }
.developer-form textarea:focus { outline: none; border-color: var(--brand); box-shadow: var(--ring); }
.developer-form .query-input { min-height: 74px; }
.developer-form .form-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); }
.developer-response { display: flex; flex-direction: column; gap: var(--s3); border-top: 1px solid var(--line); padding-top: var(--s4); }
.response-meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); font-size: 12px; }
.verify-result { border: 1px solid var(--line); border-radius: var(--r-sm); padding: var(--s4); display: flex; flex-direction: column; gap: var(--s3); }
.verify-result[data-valid="true"] { border-color: color-mix(in srgb, var(--ok) 30%, var(--line)); background: var(--ok-bg); }
.verify-result[data-valid="false"] { border-color: color-mix(in srgb, var(--err) 30%, var(--line)); background: var(--err-bg); }
.verify-result .mono { overflow-wrap: anywhere; font-size: 12px; }
.loading-panel { min-height: 200px; align-items: center; justify-content: center; }
.endpoint-description { white-space: pre-wrap; }
@media (max-width: 1199px) {
  .docs-grid, .developer-tools { grid-template-columns: 1fr; }
}
@media (max-width: 720px) {
  .developer-filters { grid-template-columns: 1fr; }
  .developer-filters button { justify-self: end; }
  .developer-form .form-pair { grid-template-columns: 1fr; }
  .developer-intro { align-items: flex-start; }
  .developer-actions { gap: var(--s2); }
  .docs-grid, .developer-tools { gap: var(--s4); }
}
`;
