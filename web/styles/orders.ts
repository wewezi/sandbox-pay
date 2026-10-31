import { css } from 'lit';

export const ordersStyles = css`
  .orders-head { display: flex; align-items: center; justify-content: space-between; gap: var(--s4); }
  .orders-head > button { flex: none; }
  .cell-note { display: block; margin-top: 6px; color: var(--muted); font-size: 11.5px; line-height: 1.5; }
  .order-number { padding: 0; font-size: 12px; text-align: left; }
  td[data-order] .cell-note { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .product-cell { min-width: 210px; max-width: 300px; overflow-wrap: anywhere; }
  .trade-number { display: inline-block; max-width: 170px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: bottom; font-size: inherit; }
  .notify-note { max-width: 170px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .notify-error { color: var(--warn); }
  td[data-source] { max-width: 180px; }
  td[data-source] > span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  td[data-source] .request-source { margin-top: 6px; line-height: 1.4; }
  .request-ip, .source-location { display: block; max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .request-ip { font-size: 11px; color: var(--text); }
  .source-location { margin-top: 2px; font-size: 11px; }
  td.acts { display: table-cell; }
  td.acts > button { margin-left: var(--s2); }
  td.acts > button:first-child { margin-left: 0; }
  @media (max-width: 480px) {
    .orders-head { align-items: flex-start; }
    .page-head p { max-width: 200px; }
  }
`;
