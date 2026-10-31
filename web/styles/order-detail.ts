import { css } from 'lit';

export const orderDetailStyles = css`
  dialog.modal {
    width: calc(100% - 48px);
    max-width: 1080px;
    max-height: calc(100svh - 48px);
    padding: 0;
    overflow: hidden;
  }
  dialog.modal[open] { gap: 0; }
  .dialog-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s4);
    padding: var(--s6);
    border-bottom: 1px solid var(--line);
    flex: none;
  }
  .heading { min-width: 0; display: flex; flex-direction: column; gap: var(--s2); }
  .eyebrow { font-size: 12px; color: var(--muted); }
  .heading h3 { font-size: 19px; font-weight: 650; overflow-wrap: anywhere; color: var(--text); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; }
  .heading-meta { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s3); }
  .heading-identity { display: inline-flex; align-items: center; gap: var(--s2); min-width: 0; }
  .heading-meta .mono { overflow-wrap: anywhere; }
  .header-actions { display: flex; align-items: center; gap: var(--s2); flex: none; }
  .close-button { padding: 8px; width: 38px; height: 38px; }
  .close-button svg { width: 18px; height: 18px; stroke: currentColor; fill: none; stroke-width: 1.6; }
  .detail-body { padding: var(--s6); display: flex; flex-direction: column; gap: var(--s5); overflow-y: auto; min-height: 0; overscroll-behavior: contain; }
  .detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s5); align-items: start; }
  .detail-grid > * { min-width: 0; }
  .detail-section { padding: var(--s5); gap: var(--s4); }
  .section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); }
  .section-heading h2 { font-size: 14px; }
  .section-count { color: var(--muted); font-size: 12px; }
  dl, dt, dd { margin: 0; }
  .detail-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s4) var(--s5); }
  .detail-field { min-width: 0; display: flex; flex-direction: column; gap: 5px; }
  .detail-field.full { grid-column: 1 / -1; }
  .request-section .detail-fields { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  dt { color: var(--muted); font-size: 12px; }
  dd { font-size: 13px; line-height: 1.65; overflow-wrap: anywhere; }
  .field-value { display: flex; align-items: flex-start; gap: var(--s2); }
  .field-value > span { min-width: 0; overflow-wrap: anywhere; }
  .copy-button { padding: 1px 6px; font-size: 11px; line-height: 18px; flex: none; }
  .missing { color: var(--muted-2); }
  .money-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--s3); }
  .money-card { padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r); background: var(--panel-2); min-width: 0; }
  .money-card dt { margin-bottom: 7px; }
  .money-card dd { font-size: clamp(18px, 2vw, 23px); font-weight: 650; line-height: 1.25; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  .money-card:first-child dd { color: var(--brand); }
  .financial-meta { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s3) var(--s6); color: var(--muted); font-size: 12px; }
  .financial-meta strong { color: var(--text); font-weight: 500; }
  .section-note { font-size: 12px; color: var(--muted); line-height: 1.7; }
  .timeline { padding: 0; margin: 0; list-style: none; display: flex; flex-direction: column; }
  .timeline li { position: relative; padding: 0 0 var(--s4) 23px; border-left: 1px solid var(--line); margin-left: 4px; }
  .timeline li:last-child { padding-bottom: 0; border-left-color: transparent; }
  .timeline li::before { content: ''; position: absolute; left: -4px; top: 5px; width: 7px; height: 7px; border-radius: 50%; background: var(--brand); outline: 4px solid var(--panel); }
  .timeline-title { font-size: 13px; font-weight: 550; }
  .timeline time { display: block; font-size: 12px; color: var(--muted); margin-top: 3px; font-variant-numeric: tabular-nums; }
  .record-list { display: flex; flex-direction: column; gap: var(--s3); }
  .record { padding: var(--s4); background: var(--panel-2); border: 1px solid var(--line-soft); border-radius: var(--r); min-width: 0; }
  .record-head { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--s2); margin-bottom: var(--s3); font-size: 13px; }
  .record .detail-fields { gap: var(--s3) var(--s5); }
  .record-error { margin-top: var(--s3); padding: var(--s3); border-radius: var(--r-sm); background: var(--err-bg); color: var(--err); font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; }
  details { margin-top: var(--s3); border-top: 1px solid var(--line); padding-top: var(--s3); }
  summary { font-size: 12px; color: var(--muted); cursor: pointer; }
  pre { font-family: var(--mono); font-size: 11.5px; line-height: 1.65; padding: var(--s3); margin: var(--s3) 0 0; background: var(--panel); border: 1px solid var(--line); border-radius: var(--r-sm); overflow-x: auto; white-space: pre-wrap; overflow-wrap: anywhere; }
  .empty-records { font-size: 13px; color: var(--muted); padding: var(--s3) 0; }
  .status-state { padding: var(--s10) var(--s5); text-align: center; display: flex; flex-direction: column; align-items: center; gap: var(--s3); }
  .status-state h2 { font-size: 15px; }
  .status-state p { font-size: 13px; color: var(--muted); max-width: 460px; overflow-wrap: anywhere; }
  .refresh-error { padding: var(--s3) var(--s4); border: 1px solid color-mix(in srgb, var(--err) 24%, var(--line)); border-radius: var(--r); background: var(--err-bg); color: var(--err); font-size: 13px; }
  .detail-footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--s3); border-top: 1px solid var(--line); padding: var(--s4) var(--s6); flex: none; }
  .detail-footer p { color: var(--muted); font-size: 12px; }
  .footer-links { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s4); font-size: 13px; }
  @media (max-width: 720px) {
    dialog.modal { width: calc(100% - 24px); max-height: calc(100svh - 24px); }
    .dialog-header, .detail-body { padding: var(--s4); }
    .dialog-header { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--s2); }
    .heading { display: contents; }
    .eyebrow { grid-column: 1; grid-row: 1; align-self: center; }
    .heading h3 { grid-column: 1 / -1; grid-row: 2; }
    .heading-meta { grid-column: 1 / -1; grid-row: 3; }
    .heading > p { grid-column: 1 / -1; }
    .detail-body, .detail-grid { gap: var(--s4); }
    .detail-grid { grid-template-columns: minmax(0, 1fr); }
    .request-section .detail-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .money-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .detail-section { padding: var(--s4); }
    .header-actions { grid-column: 2; grid-row: 1; gap: var(--s1); }
    .header-actions .refresh-button { padding: 8px 10px; }
    .detail-footer { padding: var(--s3) var(--s4); }
  }
  @media (max-width: 400px) {
    .heading h3 { font-size: 17px; }
    .heading-meta { gap: var(--s2); }
    .heading-meta .mono { font-size: 10.5px; }
    .detail-fields { gap: var(--s3); }
    .copy-button { padding: 1px 4px; }
  }
`;
