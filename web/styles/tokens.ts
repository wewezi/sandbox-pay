import { css } from 'lit';

export const tokenStyles = css`
  .tokens-head, .panel-heading, .project-heading, .modal-header {
    display: flex; align-items: center; justify-content: space-between; gap: var(--s4);
  }
  .head-actions, .project-actions { display: flex; align-items: center; gap: var(--s2); flex-wrap: wrap; }
  .tokens-panel { gap: var(--s4); }
  .panel-heading h2 { font-size: 14px; }
  .panel-heading .default-label { margin-left: 6px; font-size: 12px; }
  .project-filter { display: flex; align-items: center; gap: var(--s2); color: var(--muted); font-size: 12px; }
  .project-filter sp-select { width: 180px; min-width: 150px; }
  .token-name { font-weight: 550; }
  .token-prefix { white-space: nowrap; font-size: 12px; }
  .token-time { color: var(--muted); font-size: 12px; font-variant-numeric: tabular-nums; }
  .token-note { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.7; }
  .empty-tokens { padding: var(--s8) var(--s4); text-align: center; color: var(--muted); }
  .empty-tokens p { margin: 0 0 var(--s3); }
  .list-error { gap: var(--s3); }
  .list-error p { margin: 0; }
  .projects { display: block; }
  .projects > summary { cursor: pointer; font-size: 14px; font-weight: 600; }
  .projects-body { display: flex; flex-direction: column; gap: var(--s5); padding-top: var(--s5); }
  .project-list { display: flex; flex-direction: column; gap: var(--s3); }
  .project-item { padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r); }
  .project-title { display: flex; align-items: center; gap: var(--s2); flex-wrap: wrap; min-width: 0; }
  .project-title strong { overflow-wrap: anywhere; }
  .project-item .token-note { margin-top: var(--s2); }
  .default-label { font-size: 11px; color: var(--muted); font-weight: 400; }
  .project-form { border-top: 1px solid var(--line); padding-top: var(--s4); }
  .project-form .field { max-width: 520px; }
  dialog.token-modal { max-width: 560px; max-height: calc(100svh - 32px); overflow: auto; }
  dialog.token-modal[open] { gap: var(--s4); }
  .modal-header { align-items: center; }
  .modal-header h3 { margin: 0; }
  .modal-close { width: 36px; height: 36px; padding: 0; font-size: 20px; line-height: 1; flex: none; }
  .token-form { display: flex; flex-direction: column; gap: var(--s4); }
  .advanced { font-size: 12px; color: var(--muted); }
  .advanced > summary { cursor: pointer; margin-bottom: var(--s3); }
  .secret-box { display: block; width: 100%; padding: var(--s4); margin: 0;
    border: 1px solid var(--line); border-radius: var(--r); background: var(--panel-2);
    color: var(--text); font-size: 12px; line-height: 1.8; overflow-wrap: anywhere; white-space: pre-wrap; }
  .secret-receipt { display: flex; flex-direction: column; gap: var(--s4); }
  .receipt-note { margin: 0; font-size: 13px; line-height: 1.75; color: var(--muted); }
  .receipt-note strong { color: var(--text); }
  .connection-example { border-top: 1px solid var(--line); padding-top: var(--s4); }
  .connection-example p { margin: 0 0 var(--s2); }
  .connection-example code { font-size: 12px; overflow-wrap: anywhere; }
  .form-error { margin: 0; }
  @media (max-width: 720px) {
    .tokens-head, .panel-heading, .project-heading { align-items: flex-start; }
    .tokens-head { flex-wrap: wrap; }
    .head-actions { margin-left: auto; }
    .panel-heading, .project-heading { flex-wrap: wrap; }
    .project-actions { width: 100%; }
    .project-filter { width: 100%; justify-content: space-between; }
    .project-filter sp-select { flex: 1; max-width: 240px; }
    .project-form .field { max-width: none; }
    .project-form button { align-self: flex-end; }
  }
`;
