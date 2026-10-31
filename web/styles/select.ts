import { css } from 'lit';
import { scrollbarStyles } from './scrollbars';

export const selectStyles = css`
  ${scrollbarStyles}
  :host { display: inline-block; width: 100%; min-width: 0; color: var(--text); font: inherit; vertical-align: middle; }
  *, *::before, *::after { box-sizing: border-box; }
  [hidden] { display: none !important; }
  button { appearance: none; font: inherit; cursor: pointer; }
  .trigger {
    display: flex; align-items: center; gap: 6px; width: 100%; min-width: 0; min-height: 38px;
    padding: 8px var(--control-padding-inline, var(--s3)); border: 1px solid var(--line); border-radius: var(--r-sm);
    background: var(--panel); color: var(--text); font-size: 13.5px; line-height: 20px; text-align: left;
    transition: border-color .12s, background-color .12s, box-shadow .12s;
  }
  .trigger:hover:not(:disabled) { border-color: var(--muted-2); }
  .trigger:focus-visible, .trigger[aria-expanded="true"] { outline: none; border-color: var(--brand); box-shadow: var(--ring); }
  .trigger:disabled { cursor: not-allowed; color: var(--muted-2); background: var(--panel-2); opacity: .75; }
  .value-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .arrow { flex: none; width: 14px; height: 14px; color: var(--muted); transition: transform .12s; }
  .trigger[aria-expanded="true"] .arrow { transform: rotate(180deg); }
  .listbox {
    display: none; position: fixed; inset: auto; margin: 0; padding: 4px; overflow: auto;
    border: 1px solid var(--line); border-radius: var(--r); background: var(--panel); color: var(--text);
    box-shadow: var(--shadow-lg); z-index: 100; overscroll-behavior: contain;
  }
  .listbox:popover-open { display: block; }
  .listbox[data-fallback-open] { display: block; }
  .listbox::backdrop { background: transparent; }
  .option {
    display: flex; align-items: center; gap: var(--s2); width: 100%; min-height: 36px; padding: 8px var(--s2);
    border: none; border-radius: var(--r-sm); background: transparent; color: var(--text);
    font-size: 13px; line-height: 20px; text-align: left;
  }
  .option-label { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .option.active:not(:disabled), .option:hover:not(:disabled) { background: var(--panel-2); }
  .option[aria-selected="true"] { color: var(--brand); }
  .option[aria-selected="true"].active, .option[aria-selected="true"]:hover { background: var(--brand-soft); }
  .option:disabled { cursor: not-allowed; color: var(--muted-2); }
  .check { flex: none; width: 14px; height: 14px; opacity: 0; }
  .option[aria-selected="true"] .check { opacity: 1; }
  .empty { padding: var(--s3); color: var(--muted); font-size: 13px; text-align: center; }
  @media (prefers-reduced-motion: reduce) { .trigger, .arrow { transition: none; } }
`;
