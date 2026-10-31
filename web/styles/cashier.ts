import { css } from 'lit';

export const cashierStyles = css`
  :host { --cashier-ease: cubic-bezier(.16, 1, .3, 1); display: block; min-height: 100svh; background: radial-gradient(ellipse at 50% 0%, color-mix(in srgb, var(--brand) 5%, var(--bg)), var(--bg) 65%); }
  .cashier-page { min-height: 100svh; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 40px 24px; }
  .cashier-topbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; max-width: 1000px; margin-bottom: 24px; }
  .brand-group { display: flex; align-items: center; gap: 16px; min-width: 0; }
  .cashier-head { display: flex; align-items: center; gap: 9px; font-size: 16px; font-weight: 650; letter-spacing: -0.025em; color: var(--text); }
  .cashier-head::before { content: ''; width: 24px; height: 24px; flex: none; background: var(--brand); -webkit-mask: url('/favicon.svg') center / contain no-repeat; mask: url('/favicon.svg') center / contain no-repeat; }
  .cashier-label { padding-left: 16px; border-left: 1px solid var(--line); font-size: 12px; color: var(--muted); }
  sp-theme-toggle { flex: none; }
  .cashier-footer { margin-top: 24px; font-size: 11.5px; color: var(--muted); }
  .checkout-shell { width: 100%; background: var(--panel); border: 1px solid var(--line); border-radius: 20px; box-shadow: 0 16px 60px -32px light-dark(rgba(22, 43, 31, .22), rgba(0, 0, 0, .65)); overflow: hidden; }
  .checkout-shell { display: grid; grid-template-columns: minmax(0, .38fr) minmax(0, .62fr); max-width: 1000px; min-height: 603px; }
  .order-summary { display: flex; flex-direction: column; padding: 36px 32px; background: color-mix(in srgb, var(--brand) 2%, var(--panel-2)); border-right: 1px solid var(--line); min-width: 0; }
  .summary-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 20px; }
  .order-source { display: flex; align-items: flex-start; gap: 7px; min-width: 0; font-size: 11.5px; color: var(--muted); line-height: 1.6; }
  .order-source .icon-svg { width: 14px; height: 14px; flex: none; margin-top: 2px; }
  .order-source > span { overflow-wrap: anywhere; }
  .eyebrow, .currency-label { font-size: 11.5px; color: var(--muted); }
  .currency-label { font-variant-numeric: tabular-nums; letter-spacing: .05em; }
  .product { margin: 0; font-size: 18px; line-height: 1.6; font-weight: 550; color: var(--text); overflow-wrap: anywhere; }
  .amount-label { margin-top: 20px; margin-bottom: 6px; font-size: 12px; color: var(--muted); }
  .amount { display: flex; align-items: baseline; gap: 3px; min-width: 0; font-size: 46px; font-weight: 650; line-height: 1.15; letter-spacing: -.045em; font-variant-numeric: tabular-nums; color: var(--text); }
  .amount .currency { font-size: .55em; font-weight: 500; letter-spacing: 0; }
  .amount-long { font-size: 31px; }
  .amount > span:last-child { min-width: 0; overflow-wrap: anywhere; }
  .countdown { display: flex; align-items: center; gap: 7px; margin-top: 16px; color: var(--muted); font-size: 11.5px; font-variant-numeric: tabular-nums; line-height: 1.6; }
  .countdown .icon-svg { width: 15px; height: 15px; }
  .summary-paid { color: var(--ok); }
  .summary-fields { margin-top: 20px; padding-top: 8px; border-top: 1px solid var(--line); }
  .kv { display: flex; align-items: baseline; gap: 16px; padding: 10px 0; font-size: 12px; line-height: 1.7; }
  .kv .k { flex: none; color: var(--muted); }
  .kv .v { min-width: 0; margin-left: auto; text-align: right; overflow-wrap: anywhere; }
  .summary-fields .v, .order-details .v { font-size: 11px; }
  .summary-fields .v:not(.mono), .order-details .v:not(.mono) { font-size: 12px; }
  .summary-fields .kv, .order-details .kv { padding: 5px 0; }
  .order-details { margin-top: 8px; border-top: 1px solid var(--line); }
  summary { list-style: none; cursor: pointer; }
  summary::-webkit-details-marker { display: none; }
  .wallet-method > summary::after { content: none; }
  .order-details > h2 { margin: 0; padding: 10px 0 4px; color: var(--muted); font-size: 12px; font-weight: 500; }
  .order-details [data-details] { padding-top: 2px; }
  .copy-inline { padding: 1px 5px; margin-left: 5px; font-size: 10.5px; line-height: 16px; color: var(--brand); background: transparent; }
  .summary-note { display: flex; align-items: flex-start; gap: 8px; margin-top: auto; padding-top: 20px; font-size: 11.5px; color: var(--muted); line-height: 1.8; }
  .summary-note .icon-svg { width: 15px; height: 15px; flex: none; margin-top: 3px; }
  .payment-pane { min-width: 0; padding: 36px 40px; }
  .result-pane { display: flex; align-items: center; justify-content: center; }
  .payment-result { display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 400px; text-align: center; }
  .result-heading { display: flex; flex-direction: column; align-items: center; width: 100%; }
  .payment-result h2 { margin: 0; font-size: 24px; font-weight: 650; letter-spacing: -.025em; line-height: 1.45; color: var(--text); }
  .payment-heading { margin-bottom: 24px; }
  .payment-heading h2 { margin: 0; font-size: 21px; font-weight: 650; letter-spacing: -.02em; color: var(--text); }
  .payment-subtitle { display: flex; align-items: baseline; flex-wrap: wrap; gap: 0 8px; min-height: 20px; margin-top: 6px; color: var(--muted); font-size: 12px; line-height: 1.7; }
  .retry-inline { padding: 0; border: 0; border-radius: 0; color: var(--brand); background: transparent; font-size: inherit; line-height: inherit; }
  .retry-inline:hover { text-decoration: underline; }
  .retry-inline:focus-visible { outline: 2px solid var(--brand); outline-offset: 3px; border-radius: 2px; }
  .retry-inline:disabled { color: var(--muted); }
  .payment-methods { display: flex; flex-direction: column; gap: 10px; }
  .payment-methods:has(> .wallet-method[open]) > [data-alipay],
  .payment-methods:has(> .wallet-method[open]) > [data-paypal] { display: none; }
  .payment-method { display: flex; align-items: center; justify-content: flex-start; gap: 14px; width: 100%; min-height: 76px; padding: 15px 18px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); text-align: left; white-space: normal; color: var(--text); }
  button.payment-method:hover, .wallet-method > summary:hover { border-color: color-mix(in srgb, var(--brand) 35%, var(--line)); background: color-mix(in srgb, var(--brand) 3%, var(--panel)); }
  button.payment-method:disabled { opacity: .5; background: var(--panel); border-color: var(--line); }
  .provider-mark { display: grid; place-items: center; width: 38px; height: 38px; flex: none; border-radius: 9px; }
  .provider-mark img { display: block; object-fit: contain; }
  .provider-mark.alipay { background: light-dark(#edf5ff, #11263b); }
  .provider-mark.paypal { background: #edf3fb; }
  .provider-mark.wallet { background: var(--brand-soft); color: var(--brand); }
  .mintweft-mark { width: 24px; height: 24px; background: currentColor; -webkit-mask: url('/favicon.svg') center / contain no-repeat; mask: url('/favicon.svg') center / contain no-repeat; }
  .method-copy { display: flex; flex-direction: column; gap: 4px; min-width: 0; flex: 1; }
  .method-copy strong { font-size: 14px; font-weight: 600; line-height: 1.45; }
  .method-title { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 4px 8px; }
  .method-amount { white-space: nowrap; font-size: 12px; font-variant-numeric: tabular-nums; }
  .method-copy > span { font-size: 11.5px; font-weight: 400; color: var(--muted); line-height: 1.6; }
  .payment-method > .icon-svg, .wallet-chevron { flex: none; margin-left: auto; color: var(--muted-2); }
  .wallet-method { border: 1px solid var(--line); border-radius: 10px; overflow: hidden; }
  .wallet-method > summary { border: 0; border-radius: 0; }
  .wallet-method > summary[aria-disabled="true"] { opacity: .5; cursor: default; }
  .wallet-chevron { display: flex; transition: transform .15s; }
  .wallet-method[open] .wallet-chevron { transform: rotate(90deg); }
  .wallet-method[open] { border-color: color-mix(in srgb, var(--brand) 30%, var(--line)); }
  .wallet-method[open] > summary { background: color-mix(in srgb, var(--brand) 3%, var(--panel)); }
  .wallet-content { padding: 18px; border-top: 1px solid var(--line-soft); }
  .wallet-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  .wallet-fields label { font-size: 11.5px; font-weight: 500; }
  .wallet-fields input { min-height: 42px; border-radius: 7px; font-size: 13px; background: var(--panel-2); }
  .pin-label { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .pin-label > span { color: var(--muted); font-size: 10.5px; }
  #wallet-user-hint { white-space: nowrap; font-size: 10px; }
  .pin-entry { position: relative; min-width: 0; }
  .wallet-fields .pin-input { position: absolute; inset: 0; z-index: 1; width: 100%; height: 42px; min-height: 0; margin: 0; padding: 0; border: 0; opacity: 0; font-size: 16px; cursor: text; }
  .pin-boxes { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 6px; pointer-events: none; }
  .pin-cell { display: grid; place-items: center; height: 42px; min-width: 0; border: 1px solid var(--line); border-radius: 7px; background: var(--panel-2); }
  .pin-entry:focus-within .pin-cell.active { border-color: var(--brand); box-shadow: var(--ring); }
  .pin-entry:focus-within .pin-cell.selected { border-color: var(--brand); background: var(--brand-soft); }
  .pin-entry:focus-within .pin-cell.selected .pin-dot { background: var(--brand); }
  .pin-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--text); }
  button.primary.wallet-submit, button.primary.status-action { min-height: 44px; width: 100%; border-radius: 8px; font-size: 13px; font-weight: 550; background: var(--brand); color: light-dark(#fff, #082015); }
  button.primary.wallet-submit:hover, button.primary.status-action:hover { background: var(--brand-dark); }
  .wallet-submit { margin-top: 16px; }
  .wallet-submit .icon-svg, .status-action .icon-svg { width: 16px; height: 16px; }
  .qr-section { display: grid; grid-template-columns: 142px minmax(0, 1fr); align-items: center; gap: 24px; margin-top: 28px; padding-top: 24px; border-top: 1px solid var(--line-soft); }
  .qr-surface { padding: 8px; border: 1px solid var(--line); border-radius: 10px; background: #fff; }
  .qr { width: 124px; height: 124px; display: block; background: #fff; }
  .qr-frame { position: relative; width: 124px; height: 124px; }
  .qr-pending { opacity: 0; }
  .qr-feedback { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 7px; color: #68746b; font-size: 10.5px; text-align: center; }
  .qr-feedback .retry-inline { color: #21854d; }
  .qr-copy h3 { margin: 0 0 8px; color: var(--text); font-size: 13px; font-weight: 600; }
  .qr-copy p { color: var(--muted); font-size: 11.5px; line-height: 1.8; overflow-wrap: anywhere; }
  .icon-svg { display: block; width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 1.6; stroke-linecap: round; stroke-linejoin: round; }
  summary:focus-visible { outline: 2px solid var(--brand); outline-offset: -3px; border-radius: 8px; }
  .status-icon { display: grid; place-items: center; width: 56px; height: 56px; border-radius: 50%; margin-bottom: 20px; }
  .status-icon .icon-svg { width: 30px; height: 30px; stroke-width: 1.8; }
  .status-icon.success { color: var(--ok); background: var(--ok-bg); }
  .status-icon.warning { color: var(--warn); background: var(--warn-bg); }
  .status-icon.error { color: var(--err); background: var(--err-bg); }
  .status-description { font-size: 12px; color: var(--muted); line-height: 1.8; margin-top: 10px; max-width: 360px; overflow-wrap: anywhere; }
  .receipt-fields { width: 100%; margin-top: 24px; padding-top: 14px; border-top: 1px solid var(--line-soft); }
  .receipt-fields .kv { text-align: left; padding: 8px 0; }
  .receipt-fields .v { font-size: 11.5px; }
  .status-action { margin-top: 28px; }
  .loading-status-dot { width: 6px; height: 6px; flex: none; border-radius: 50%; background: var(--brand); }
  .skeleton { position: relative; display: block; flex: none; overflow: hidden; border-radius: 5px; background: color-mix(in srgb, var(--muted) 14%, var(--panel)); }
  .checkout-loading .skeleton::after { content: ''; position: absolute; inset: 0; background: linear-gradient(100deg, transparent 15%, color-mix(in srgb, var(--text) 6%, transparent) 50%, transparent 85%); transform: translateX(-100%); animation: cashier-shimmer 2.2s ease-in-out infinite; }
  .skeleton-caption { width: 60px; height: 11px; }
  .skeleton-currency { width: 28px; height: 11px; }
  .skeleton-product { width: 74%; height: 22px; margin: 3px 0 4px; }
  .skeleton-summary .amount-label { display: flex; align-items: center; min-height: 18px; }
  .skeleton-amount { width: 64%; max-width: 210px; height: 53px; border-radius: 7px; }
  .skeleton-summary .countdown { min-height: 19px; }
  .skeleton-dot { width: 12px; height: 12px; border-radius: 50%; }
  .skeleton-time { width: 132px; height: 11px; }
  .skeleton-row { align-items: center; min-height: 40px; }
  .skeleton-value { width: 116px; height: 10px; margin-left: auto; }
  .skeleton-details { display: flex; align-items: center; justify-content: space-between; min-height: 46px; }
  .skeleton-note { display: flex; flex-direction: column; gap: 8px; flex: 1; padding-top: 4px; }
  .skeleton-text { width: 100%; height: 10px; }
  .skeleton-text-short { width: 68%; height: 10px; }
  .skeleton-provider { width: 38px; height: 38px; border-radius: 9px; }
  .skeleton-method .method-copy { gap: 8px; }
  .skeleton-method-title { width: 88px; height: 12px; }
  .skeleton-method-description { width: 58%; max-width: 188px; height: 10px; }
  .skeleton-method > .skeleton-dot { width: 10px; height: 10px; margin-left: auto; }
  .skeleton-qr-surface { background: var(--panel); }
  .skeleton-qr { border-radius: 4px; }
  .skeleton-qr-copy { display: flex; flex-direction: column; gap: 11px; }
  .cashier-topbar { animation: cashier-rise .34s var(--cashier-ease) backwards; }
  .checkout-shell { animation: cashier-rise .42s var(--cashier-ease) backwards; transition: background-color .2s ease, border-color .2s ease; }
  .order-summary { transition: background-color .2s ease, border-color .2s ease; }
  .order-summary .amount { animation: cashier-rise .42s var(--cashier-ease) .06s backwards; }
  .cashier-footer { animation: cashier-rise .34s var(--cashier-ease) .12s backwards; }
  .payment-methods > * { animation: cashier-rise .32s var(--cashier-ease) backwards; }
  .payment-methods > :nth-child(1) { animation-delay: .04s; }
  .payment-methods > :nth-child(2) { animation-delay: .08s; }
  .payment-methods > :nth-child(3) { animation-delay: .12s; }
  .payment-method, .wallet-method { transition: background-color .18s ease, border-color .18s ease, box-shadow .18s ease, transform .18s var(--cashier-ease), opacity .18s ease; }
  .provider-mark, .payment-method > .icon-svg, .status-action > .icon-svg, .wallet-submit > .icon-svg { transition: transform .18s var(--cashier-ease); }
  .wallet-chevron { transition: transform .22s var(--cashier-ease); }
  .wallet-method[open] { animation: none; }
  .wallet-method[open] .wallet-content { animation: cashier-rise .24s var(--cashier-ease) backwards; }
  .wallet-content:focus-within, .payment-method:focus-visible { animation: none !important; }
  .wallet-method:has(> summary:focus-visible) { animation: none; }
  .pin-cell { transition: border-color .14s ease, box-shadow .14s ease, background-color .18s ease; }
  .pin-dot { animation: cashier-dot .14s var(--cashier-ease) backwards; }
  .wallet-submit, .status-action { transition: background-color .18s ease, box-shadow .18s ease, transform .16s var(--cashier-ease), opacity .18s ease; }
  .wallet-submit:not(:disabled) { animation: cashier-ready .36s ease; }
  .payment-result { animation: cashier-rise .32s var(--cashier-ease) backwards; }
  .payment-result .receipt-fields { animation: cashier-rise .32s var(--cashier-ease) .10s backwards; }
  .payment-result .status-action { animation: cashier-rise .32s var(--cashier-ease) .16s backwards; }
  .payment-result .status-action:focus-visible { animation: none; }
  .status-icon { position: relative; animation: cashier-pop .38s var(--cashier-ease) backwards; }
  .status-icon.success::after { content: ''; position: absolute; inset: -4px; border: 1px solid var(--ok); border-radius: inherit; opacity: 0; pointer-events: none; animation: cashier-ring .56s ease .14s; }
  .result-paid .status-icon .icon-svg path { stroke-dasharray: 1; stroke-dashoffset: 0; animation: cashier-check .38s ease .12s backwards; }
  .action-spinner { width: 16px; height: 16px; flex: none; border: 2px solid currentColor; border-right-color: transparent; border-radius: 50%; animation: cashier-spin .7s linear infinite; }
  .payment-method > .action-spinner { margin-left: auto; color: var(--muted); }
  @keyframes cashier-rise { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
  @keyframes cashier-pop { from { opacity: 0; transform: scale(.82); } to { opacity: 1; transform: scale(1); } }
  @keyframes cashier-dot { from { opacity: 0; transform: scale(.4); } to { opacity: 1; transform: scale(1); } }
  @keyframes cashier-check { from { stroke-dashoffset: 1; } to { stroke-dashoffset: 0; } }
  @keyframes cashier-ring { from { opacity: .45; transform: scale(.9); } to { opacity: 0; transform: scale(1.32); } }
  @keyframes cashier-ready { from { box-shadow: 0 0 0 4px color-mix(in srgb, var(--brand) 18%, transparent); } to { box-shadow: 0 0 0 8px transparent; } }
  @keyframes cashier-shimmer { 0%, 15% { transform: translateX(-100%); } 70%, 100% { transform: translateX(100%); } }
  @media (hover: hover) and (prefers-reduced-motion: no-preference) {
    button.payment-method:not(:disabled):hover { transform: translateY(-2px); box-shadow: 0 5px 16px -10px color-mix(in srgb, var(--brand) 30%, transparent); }
    button.payment-method:not(:disabled):hover .provider-mark, .wallet-method > summary:hover .provider-mark { transform: scale(1.06); }
    button.payment-method:not(:disabled):hover > .icon-svg, .wallet-submit:not(:disabled):hover > .icon-svg, .status-action:hover > .icon-svg { transform: translateX(3px); }
    .wallet-submit:not(:disabled):hover, .status-action:hover { transform: translateY(-1px); }
    button.payment-method:not(:disabled):active, .wallet-submit:not(:disabled):active, .status-action:active { transform: translateY(0) scale(.99); }
  }
  @keyframes cashier-spin { to { transform: rotate(360deg); } }
  @media (max-width: 900px) {
    .checkout-shell { grid-template-columns: minmax(0, .4fr) minmax(0, .6fr); }
    .order-summary { padding: 28px 24px; }
    .payment-pane { padding: 28px; }
    .amount { font-size: 40px; }
    .skeleton-amount { height: 46px; }
    .amount-long { font-size: 26px; }
    .qr-section { grid-template-columns: 124px minmax(0, 1fr); gap: 16px; }
    .qr, .qr-frame { width: 106px; height: 106px; }
    .pin-boxes { gap: 4px; }
  }
  @media (max-width: 700px) {
    .cashier-page { justify-content: flex-start; padding: 24px 16px; }
    .cashier-topbar { margin-bottom: 18px; }
    .cashier-head { font-size: 15px; }
    .checkout-shell { max-width: 460px; grid-template-columns: minmax(0, 1fr); border-radius: 16px; min-height: 0; }
    .order-summary { border-right: 0; border-bottom: 1px solid var(--line); padding: 24px; }
    .summary-heading { margin-bottom: 14px; }
    .product { font-size: 16px; line-height: 1.55; }
    .amount-label { margin-top: 20px; }
    .amount { font-size: 39px; }
    .skeleton-amount { height: 45px; }
    .amount-long { font-size: 29px; }
    .countdown { margin-top: 12px; }
    .summary-fields { margin-top: 20px; padding-top: 8px; }
    .order-details { margin-top: 4px; }
    .order-details > h2 { padding: 12px 0 6px; }
    .summary-note { display: none; }
    .payment-pane { padding: 24px; }
    .result-pane { padding-top: 32px; padding-bottom: 32px; }
    .payment-result h2 { font-size: 22px; }
    .payment-heading { margin-bottom: 18px; }
    .payment-heading h2 { font-size: 18px; }
    .payment-method { min-height: 72px; padding: 14px; gap: 12px; }
    .provider-mark { width: 34px; height: 34px; }
    .skeleton-provider { width: 34px; height: 34px; }
    .method-copy strong { font-size: 13px; }
    .method-copy > span { font-size: 11px; }
    .wallet-fields { grid-template-columns: minmax(0, 1fr); }
    .wallet-fields input { font-size: 16px; }
    .qr-section { margin-top: 24px; padding-top: 20px; }
    .cashier-footer { margin-top: 20px; }
  }
  @media (max-width: 360px) {
    .cashier-label { display: none; }
    .order-summary, .payment-pane { padding: 20px; }
    .amount { font-size: 36px; }
    .skeleton-amount { height: 42px; }
    .amount-long { font-size: 25px; }
    .qr-section { grid-template-columns: 104px minmax(0, 1fr); gap: 14px; }
    .qr, .qr-frame { width: 86px; height: 86px; }
  }
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after { animation: none !important; transition: none !important; }
  }
`;
