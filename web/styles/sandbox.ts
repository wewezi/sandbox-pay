import { css } from 'lit';

export const sandboxStyles = css`
  .sandbox-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: var(--s5); align-items: stretch; }
  .sandbox-sidebar { min-width: 0; display: grid; grid-template-rows: auto minmax(0, 1fr); gap: var(--s5); }
  .order-preview { min-width: 0; gap: var(--s5); }
  .preview-head { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); }
  .preview-tag { font-size: 11px; line-height: 1.6; color: var(--muted); border: 1px solid var(--line); border-radius: var(--r-full); padding: 2px 8px; white-space: nowrap; }
  .preview-total { display: flex; flex-direction: column; gap: 8px; }
  .preview-total > span { color: var(--muted); font-size: 12px; }
  .preview-amount { color: var(--text); font-size: 34px; font-weight: 650; letter-spacing: -0.04em; line-height: 1.2; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  .preview-amount.is-empty { color: var(--muted); font-size: 20px; letter-spacing: 0; }
  .preview-product { color: var(--text); font-size: 13px; line-height: 1.65; overflow-wrap: anywhere; }
  .preview-facts { display: flex; flex-direction: column; gap: var(--s3); margin: 0; padding-top: var(--s5); border-top: 1px solid var(--line-soft); }
  .preview-facts > div { display: grid; grid-template-columns: 62px minmax(0, 1fr); gap: var(--s3); align-items: baseline; font-size: 12px; line-height: 1.65; }
  .preview-facts dt { color: var(--muted); }
  .preview-facts dd { min-width: 0; margin: 0; text-align: right; overflow-wrap: anywhere; }
  .preview-destinations { display: flex; flex-direction: column; gap: var(--s4); padding-top: var(--s5); border-top: 1px solid var(--line-soft); }
  .preview-destination { min-width: 0; }
  .preview-destination > div { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); color: var(--muted); font-size: 12px; }
  .preview-address-state { color: var(--muted); font-size: 11px; }
  .preview-address-state.filled { color: var(--brand); }
  .preview-url { margin-top: 5px; color: var(--muted); font-size: 11px; line-height: 1.65; overflow-wrap: anywhere; }
  .preview-note { margin-top: auto; padding-top: var(--s4); border-top: 1px solid var(--line-soft); color: var(--muted); font-size: 12px; line-height: 1.7; }
  .test-panel { gap: var(--s6); }
  .section-copy { color: var(--muted); font-size: 13px; line-height: 1.7; }
  .section-title { display: flex; flex-direction: column; gap: 6px; }
  .test-form { display: flex; flex-direction: column; gap: var(--s5); }
  .form-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: var(--s5); }
  .basics-grid { grid-template-columns: minmax(0, 1fr) 180px; }
  .field-note { font-size: 12px; color: var(--muted); line-height: 1.6; }
  .test-form input[readonly] { color: var(--muted); background: var(--panel-2); cursor: default; }
  .form-section { border-top: 1px solid var(--line-soft); padding-top: var(--s5); display: flex; flex-direction: column; gap: var(--s4); }
  .form-section h3 { color: var(--text); }
  .submit-row { display: flex; justify-content: space-between; align-items: center; gap: var(--s4); flex-wrap: wrap; }
  .submit-row span { font-size: 12px; color: var(--muted); line-height: 1.65; }
  .steps { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: var(--s5); counter-reset: step; }
  .steps li { display: grid; grid-template-columns: 26px minmax(0, 1fr); gap: var(--s3); align-items: start; counter-increment: step; }
  .steps li::before { content: counter(step); display: grid; place-items: center; height: 26px; background: var(--brand-soft); color: var(--brand); border-radius: 50%; font-size: 12px; font-weight: 600; }
  .step-copy { display: flex; flex-direction: column; gap: 5px; }
  .step-copy strong { font-size: 13px; font-weight: 600; }
  .step-copy p { color: var(--muted); font-size: 12px; line-height: 1.7; }
  .reset-panel { flex-direction: row; align-items: center; justify-content: space-between; gap: var(--s5); }
  .reset-panel h2 { margin-bottom: 6px; }
  .receipt-title { display: flex; align-items: center; gap: var(--s3); }
  .receipt-check { width: 32px; height: 32px; flex-shrink: 0; display: grid; place-items: center; border-radius: 50%; color: var(--brand); background: var(--brand-soft); font-size: 20px; }
  .receipt-check.paid { color: var(--ok); background: var(--ok-bg); }
  .receipt-check.failed { color: var(--err); background: var(--err-bg); }
  .receipt-check.expired { color: var(--muted); background: var(--panel-2); }
  .receipt-number { display: flex; align-items: center; gap: var(--s3); flex-wrap: wrap; background: var(--panel-2); padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r); }
  .receipt-number code { font-family: var(--mono); font-size: 14px; overflow-wrap: anywhere; }
  .receipt-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s5); margin: 0; }
  .receipt-grid div { min-width: 0; }
  .receipt-grid dt { font-size: 12px; color: var(--muted); margin-bottom: 6px; }
  .receipt-grid dd { margin: 0; font-size: 13px; overflow-wrap: anywhere; }
  .receipt-grid .full { grid-column: 1 / -1; }
  .receipt-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); border-top: 1px solid var(--line-soft); padding-top: var(--s5); }
  .action-link { display: inline-flex; justify-content: center; align-items: center; padding: 8px var(--s4); font-size: 13.5px; line-height: 20px; border: 1px solid var(--line); border-radius: var(--r-sm); color: var(--text); text-decoration: none; }
  .action-link:hover { background: var(--line-soft); text-decoration: none; }
  .action-link.primary { color: var(--on-brand); background: var(--brand); border-color: var(--brand); }
  .action-link.primary:hover { background: var(--brand-dark); }
  .inline-error { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s2); }
  .scenario-form { display: flex; align-items: end; flex-wrap: wrap; gap: var(--s3); }
  .scenario-form .field { flex: 1 1 180px; }
  .scenario-form .scenario-order { flex: 2 1 300px; }
  .scenario-form .scenario-small { flex: 0 1 150px; }
  .scenario-panel h3 { color: var(--text); }
  .scenario-panel td { max-width: 320px; overflow-wrap: anywhere; }
  @media (max-width: 720px) { .scenario-form .field, .scenario-form .scenario-small { flex: 1 1 160px; } .scenario-form .scenario-order { flex-basis: 100%; } }
  @media (max-width: 1050px) {
    .sandbox-layout { grid-template-columns: minmax(0, 1fr); }
    .sandbox-sidebar { grid-template-rows: auto; }
    .steps { flex-direction: row; }
    .steps li { flex: 1; }
    .preview-amount { font-size: 30px; }
  }
  @media (max-width: 720px) {
    .test-panel, .test-form { gap: var(--s4); }
    .form-grid, .basics-grid { grid-template-columns: minmax(0, 1fr); gap: var(--s4); }
    .form-section { padding-top: var(--s4); }
    .submit-row { flex-direction: column; align-items: stretch; gap: var(--s3); }
    .steps { flex-direction: column; gap: var(--s4); }
    .reset-panel { flex-direction: column; align-items: stretch; }
  }
  @media (max-width: 440px) { .receipt-grid { grid-template-columns: minmax(0, 1fr); } .receipt-actions > * { flex: 1 1 auto; } }
`;
