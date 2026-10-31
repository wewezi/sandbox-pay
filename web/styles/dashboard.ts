import { css } from 'lit';

export const dashboardStyles = css`
  .dashboard-head { display: flex; align-items: center; justify-content: space-between; gap: var(--s4); }
  .page-head { gap: 5px; }
  .head-actions { display: flex; align-items: center; gap: var(--s3); }
  .segmented { display: inline-flex; gap: 3px; padding: 3px; background: var(--panel-2); border: 1px solid var(--line); border-radius: var(--r); }
  .segmented button { padding: 4px 12px; font-size: 12px; border: 1px solid transparent; background: transparent; color: var(--muted); }
  .segmented button[aria-pressed="true"] { color: var(--text); background: var(--panel); border-color: var(--line); }
  .dashboard-stats { grid-template-columns: repeat(6, minmax(0, 1fr)); }
  .stat { padding: var(--s5); gap: 10px; min-width: 0; }
  .stat:first-child { border-color: color-mix(in srgb, var(--brand) 30%, var(--line)); }
  .stat .k { display: flex; align-items: center; justify-content: space-between; }
  .stat:first-child .v { color: var(--brand); }
  .stat .v { font-size: clamp(22px, 2.2vw, 28px); white-space: nowrap; }
  .stat .hint { font-size: 12px; color: var(--muted); line-height: 1.6; }
  .comparison { font-size: 11.5px; color: var(--muted); margin-top: auto; padding-top: 10px; border-top: 1px solid var(--line-soft); }
  .comparison.positive { color: var(--ok); }
  .comparison.negative { color: var(--err); }
  .stat-icon { color: var(--muted-2); width: 17px; height: 17px; fill: none; stroke: currentColor; stroke-width: 1.6; stroke-linecap: round; stroke-linejoin: round; }
  .primary-grid { display: grid; grid-template-columns: minmax(0, 1fr) 310px; gap: var(--s5); align-items: stretch; }
  .primary-grid > *, .lower-grid > * { min-width: 0; }
  .panel-title { display: flex; flex-direction: column; gap: 3px; }
  .panel-title p { font-size: 12px; color: var(--muted); }
  .trend-panel { gap: var(--s5); }
  .trend-head { align-items: flex-start; flex-wrap: wrap; }
  .chart-readout { display: flex; align-items: baseline; flex-wrap: wrap; gap: 6px 14px; min-height: 27px; }
  .chart-date { color: var(--muted); font-size: 12px; }
  .chart-amount { font-size: 20px; font-weight: 650; font-variant-numeric: tabular-nums; letter-spacing: -0.02em; }
  .chart-count { color: var(--muted); font-size: 12px; }
  .chart-grid { display: grid; grid-template-columns: 58px minmax(0, 1fr); gap: 10px; }
  .y-axis { display: flex; flex-direction: column; justify-content: space-between; text-align: right; color: var(--muted-2); font-size: 11px; font-variant-numeric: tabular-nums; height: 210px; padding: 7px 0 11px; }
  .chart { width: 100%; height: 210px; overflow: visible; display: block; }
  .chart-line { fill: none; stroke: var(--brand); stroke-width: 2.2; vector-effect: non-scaling-stroke; }
  .chart-area { fill: url(#dashboard-trend-fill); }
  .chart-gridline { stroke: var(--line-soft); vector-effect: non-scaling-stroke; }
  .chart-hit { fill: transparent; cursor: pointer; outline: none; }
  .chart-hit:focus-visible { fill: color-mix(in srgb, var(--brand) 9%, transparent); }
  .chart-guide { stroke: var(--line); stroke-dasharray: 4 4; vector-effect: non-scaling-stroke; pointer-events: none; }
  .chart-point { fill: var(--panel); stroke: var(--brand); stroke-width: 2; vector-effect: non-scaling-stroke; pointer-events: none; }
  .x-axis { display: flex; justify-content: space-between; color: var(--muted-2); font-size: 11px; padding-top: 7px; }
  .chart-footnote { font-size: 11.5px; color: var(--muted); }
  .task-panel { gap: 0; padding-bottom: var(--s4); }
  .task-panel .panel-head { margin-bottom: var(--s4); }
  .task-panel .badge { font-size: 11px; }
  .task { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); padding: 13px 0; color: var(--text); border-top: 1px solid var(--line-soft); text-decoration: none; }
  .task:hover { text-decoration: none; }
  .task:hover .task-label { color: var(--brand); }
  .task-info { display: flex; flex-direction: column; gap: 2px; }
  .task-label { font-size: 13px; transition: color 0.12s; }
  .task-detail { font-size: 11.5px; color: var(--muted); }
  .task-value { display: flex; align-items: center; gap: var(--s3); color: var(--muted-2); }
  .task-value strong { font-size: 20px; font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; }
  .task.attention .task-value strong { color: var(--warn); }
  .task.alert .task-value strong { color: var(--err); }
  .task-value span { font-size: 17px; }
  /* 两栏都按自身内容高度收尾：强制等高会在渠道表下方留出一块
     有边框的空白，看起来像数据没加载出来。 */
  .lower-grid { display: grid; grid-template-columns: minmax(0, 1fr) 310px; gap: var(--s5); align-items: start; }
  .channel-label { display: inline-flex; align-items: center; gap: 9px; }
  .channel-dot { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--brand); }
  .channel-dot.paypal { background: #5295ef; }
  .channel-dot.alipay { background: #a685ec; }
  .channel-link { color: var(--text); }
  .channel-link:hover { color: var(--brand); }
  .channel-share { min-width: 68px; }
  .share-bar { display: block; width: 68px; height: 5px; margin-top: 5px; }
  .share-track { fill: var(--line-soft); }
  .share-value { fill: var(--brand); }
  .connection-card { display: flex; align-items: center; justify-content: space-between; padding: var(--s4); background: var(--panel-2); border: 1px solid var(--line-soft); border-radius: var(--r); color: var(--text); text-decoration: none; }
  .connection-card:hover { text-decoration: none; border-color: var(--line); }
  .connection-card strong { font-size: 24px; line-height: 1; font-weight: 650; font-variant-numeric: tabular-nums; }
  .connection-card span { font-size: 12px; color: var(--muted); }
  .connection-note { font-size: 11.5px; color: var(--muted); line-height: 1.7; }
  .recent-panel { gap: var(--s3); }
  .recent-no { color: var(--text); }
  .recent-no:hover { color: var(--brand); }
  .recent-product { display: inline-block; max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: bottom; }
  .recent-panel .channel-label { gap: 7px; }
  .dashboard-footer { display: flex; justify-content: space-between; flex-wrap: wrap; gap: var(--s2); font-size: 11.5px; color: var(--muted); }
  .dashboard-status { padding: var(--s10) var(--s6); align-items: center; text-align: center; gap: var(--s3); }
  .status-mark { display: grid; place-items: center; width: 44px; height: 44px; border-radius: var(--r-full); color: var(--brand); background: var(--brand-soft); font-size: 23px; }
  .status-mark.error { color: var(--err); background: var(--err-bg); }
  .status-mark.loading { animation: pulse 1.3s ease-in-out infinite; }
  .status-copy { color: var(--muted); font-size: 13px; max-width: 420px; overflow-wrap: anywhere; }
  .empty-channel { padding: var(--s6); color: var(--muted); font-size: 12px; text-align: center; }
  @keyframes pulse { 50% { opacity: 0.4; } }
  @media (max-width: 1700px) {
    .dashboard-stats { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  }
  @media (max-width: 1100px) {
    .primary-grid, .lower-grid { grid-template-columns: minmax(0, 1fr) 270px; }
    .stat { padding: var(--s4); }
  }
  @media (max-width: 960px) {
    .primary-grid, .lower-grid { grid-template-columns: minmax(0, 1fr); }
    .dashboard-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .task-panel { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--s5); }
    .task-panel .panel-head { grid-column: 1 / -1; }
    .connection-panel { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .connection-panel .panel-head, .connection-note { grid-column: 1 / -1; }
  }
  @media (max-width: 480px) {
    .dashboard-head { align-items: flex-start; flex-direction: column; gap: var(--s3); }
    .head-actions { width: 100%; justify-content: space-between; }
    .stat .v { font-size: clamp(18px, 5.1vw, 23px); }
    .stat .hint, .comparison { font-size: 11px; }
    .task-panel { grid-template-columns: minmax(0, 1fr); }
    .chart-grid { grid-template-columns: 48px minmax(0, 1fr); gap: 5px; }
    .chart, .y-axis { height: 180px; }
    .channel-panel .table-scroll { margin-top: -4px; }
    .channel-panel th, .channel-panel td { padding: 10px 12px; }
    .chart-count { flex-basis: 100%; }
  }
  @media (max-width: 360px) {
    .dashboard-stats { gap: var(--s3); }
    .stat { padding: var(--s3); }
  }
`;
