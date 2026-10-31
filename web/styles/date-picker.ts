import { css } from 'lit';
import { scrollbarStyles } from './scrollbars';

export const datePickerStyles = css`
  ${scrollbarStyles}
  :host { display: block; min-width: 0; width: 100%; color: var(--text); font: inherit; }
  *, *::before, *::after { box-sizing: border-box; }
  button { font: inherit; cursor: pointer; color: inherit; }
  button:disabled { cursor: not-allowed; }
  .control {
    display: flex; align-items: center; gap: 6px; width: 100%; height: 38px;
    padding: 8px var(--control-padding-inline, 12px); border: 1px solid var(--line);
    border-radius: var(--r-sm); background: var(--panel); font-size: 13.5px; line-height: 20px;
    text-align: left; transition: border-color .12s, box-shadow .12s, background .12s;
  }
  .control:hover:not(:disabled) { border-color: var(--muted-2); }
  .control:focus-visible, .control[aria-expanded="true"] { outline: none; border-color: var(--brand); box-shadow: var(--ring); }
  .control:disabled { color: var(--muted-2); background: var(--panel-2); opacity: .7; }
  .value { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .placeholder { color: var(--muted); }
  .control svg { width: 15px; height: 15px; flex: none; color: var(--muted); }
  .calendar {
    display: none; position: fixed; inset: auto; margin: 0; width: min(288px, calc(100vw - 16px));
    max-height: calc(100dvh - 16px); overflow: auto; padding: 12px;
    color: var(--text); font: inherit; background: var(--panel); border: 1px solid var(--line);
    border-radius: var(--r-lg); box-shadow: var(--shadow-lg); flex-direction: column; gap: 12px;
  }
  .calendar:popover-open { display: flex; }
  .calendar[data-fallback-open] { display: flex; z-index: 1000; }
  .calendar::backdrop { background: transparent; }
  .month { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .month-title { font-size: 14px; font-weight: 650; }
  .month button {
    display: inline-flex; align-items: center; justify-content: center; flex: none;
    width: 30px; height: 30px; padding: 0; border: 1px solid transparent;
    border-radius: var(--r-sm); background: transparent; color: var(--muted);
  }
  .month button svg { width: 16px; height: 16px; }
  .month button:disabled { opacity: .3; }
  .month button:hover:not(:disabled) { color: var(--text); background: var(--panel-2); }
  .month button:focus-visible, .footer button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .days { width: 100%; border-collapse: collapse; table-layout: fixed; }
  .days th { height: 28px; padding: 0; color: var(--muted); font-size: 11.5px; font-weight: 500; text-align: center; }
  .days td { padding: 2px; text-align: center; }
  .day {
    position: relative; display: flex; align-items: center; justify-content: center;
    width: 100%; height: 30px; border: 1px solid transparent; border-radius: var(--r-sm);
    padding: 0; background: transparent; font-size: 12.5px; line-height: 1;
  }
  .day.other-month { color: var(--muted-2); }
  .day.today:not(.selected)::after { content: ''; position: absolute; bottom: 3px; width: 3px; height: 3px; border-radius: 50%; background: var(--brand); }
  .day:hover:not(:disabled):not(.selected) { background: var(--brand-soft); color: var(--brand); }
  .day.selected { background: var(--brand); color: var(--on-brand); font-weight: 650; }
  .day:focus-visible { outline: none; border-color: var(--brand); box-shadow: var(--ring); z-index: 1; }
  .day:disabled { color: var(--muted-2); opacity: .35; }
  .footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; border-top: 1px solid var(--line-soft); padding-top: 10px; }
  .footer button { border: 0; background: transparent; color: var(--brand); font-size: 12.5px; padding: 4px 8px; border-radius: var(--r-sm); }
  .footer button:hover:not(:disabled) { background: var(--brand-soft); }
  .footer button:disabled { color: var(--muted-2); opacity: .5; }
  @media (forced-colors: active) { .day.selected { border-color: Highlight; color: Highlight; } .control, .calendar { border-color: ButtonText; } }
  @media (prefers-reduced-motion: reduce) { .control { transition: none; } }
`;
