import { css } from 'lit';
import { scrollbarStyles } from './scrollbars';

export const sharedStyles = css`
${scrollbarStyles}
:host { color: var(--text); font: inherit; }

*,
*::before,
*::after {
  box-sizing: border-box;
}


[hidden] {
  display: none !important;
}

.panel {
  min-width: 0;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  padding: var(--s6);
  display: flex;
  flex-direction: column;
  gap: var(--s4);
}

h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 650;
  letter-spacing: -0.01em;
}


.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s3);
}

.panel-head .row {

  align-items: center;
}

h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--muted);
}

p {
  margin: 0;
}

button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px var(--s4);
  font: inherit;
  font-size: 13.5px;
  font-weight: 500;

  line-height: 20px;
  white-space: nowrap;
  color: var(--text);
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-sm);
  cursor: pointer;
  transition: background-color 0.12s, border-color 0.12s, opacity 0.12s;
}

button:hover {
  background: var(--line-soft);
}


button:active {
  opacity: 0.85;
}

button:focus-visible {
  outline: none;
  border-color: var(--brand);
  box-shadow: var(--ring);
}

button:disabled {
  color: var(--muted-2);
  background: var(--panel);
  cursor: not-allowed;
}

button.primary {
  color: var(--on-brand);
  background: var(--brand);
  border-color: var(--brand);
}

button.primary:hover {
  background: var(--brand-dark);
  border-color: var(--brand-dark);
}

button.danger {
  color: var(--err);
  border-color: color-mix(in srgb, var(--err) 35%, var(--line));
}

button.danger:hover {
  background: var(--err-bg);
}


button.link {
  padding: var(--s2);
  color: var(--muted);
  background: none;
  border: none;
  font-size: 13.5px;
}

button.link:hover {
  color: var(--text);
  background: var(--line-soft);
}

.row {

  align-items: center;
}

h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--muted);
}

p {
  margin: 0;
}


.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--s4);
}

.stat {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  padding: var(--s5) var(--s5) var(--s4);
  display: flex;
  flex-direction: column;
  gap: var(--s2);
  font-size: 12.5px;
  font-weight: 500;
  color: var(--muted);
}

.stat .k {
  letter-spacing: 0.01em;
}

.stat .v {
  font-size: 26px;
  font-weight: 650;
  line-height: 1.15;
  letter-spacing: -0.025em;
  color: var(--text);

  font-variant-numeric: tabular-nums;
}


button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px var(--s4);
  font: inherit;
  font-size: 13.5px;
  font-weight: 500;

  line-height: 20px;
  white-space: nowrap;
  color: var(--text);
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-sm);
  cursor: pointer;
  transition: background-color 0.12s, border-color 0.12s, opacity 0.12s;
}

button:hover {
  background: var(--line-soft);
}


button:active {
  opacity: 0.85;
}

button:focus-visible {
  outline: none;
  border-color: var(--brand);
  box-shadow: var(--ring);
}

button:disabled {
  color: var(--muted-2);
  background: var(--panel);
  cursor: not-allowed;
}

button.primary {
  color: var(--on-brand);
  background: var(--brand);
  border-color: var(--brand);
}

button.primary:hover {
  background: var(--brand-dark);
  border-color: var(--brand-dark);
}

button.danger {
  color: var(--err);
  border-color: color-mix(in srgb, var(--err) 35%, var(--line));
}

button.danger:hover {
  background: var(--err-bg);
}


button.link {
  padding: var(--s2);
  color: var(--muted);
  background: none;
  border: none;
  font-size: 13.5px;
}

button.link:hover {
  color: var(--text);
  background: var(--line-soft);
}


.theme-toggle {
  display: flex;
  align-items: center;
  gap: var(--s3);
  cursor: pointer;
}

.theme-toggle::before {
  content: "";
  width: 17px;
  height: 17px;
  flex: none;
  background: currentColor;
  -webkit-mask: var(--icon) center / contain no-repeat;
  mask: var(--icon) center / contain no-repeat;
}


.theme-toggle[data-mode="light"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round'><circle cx='12' cy='12' r='4'/><path d='M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M6.3 17.7l-1.4 1.4M19.1 4.9l-1.4 1.4'/></svg>");
}


.theme-toggle[data-mode="dark"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><path d='M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z'/></svg>");
}


.theme-toggle[data-mode="system"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><rect x='2' y='3' width='20' height='14' rx='2'/><path d='M8 21h8M12 17v4'/></svg>");
}


.theme-toggle.floating {
  position: fixed;
  top: var(--s4);
  left: var(--s4);
  z-index: 10;
  border-radius: var(--r-sm);
}


.row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--s3);
}

.stack {
  display: flex;
  flex-direction: column;
  gap: var(--s4);
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}


.grow {
  flex: 1;
  min-width: 0;
}

.field.grow {
  min-width: 180px;
}

.field.sm {
  width: 140px;
  flex: none;
}

label {
  font-size: 12px;
  font-weight: 550;
  color: var(--muted);
}

input,
select {
  width: 100%;
  padding: 8px var(--s3);
  font: inherit;
  font-size: 13.5px;

  line-height: 20px;
  color: var(--text);
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-sm);
  transition: border-color 0.12s, box-shadow 0.12s;
}


input::placeholder {
  color: var(--muted);
}

button,
input,
select,
summary,
a {
  -webkit-tap-highlight-color: transparent;
}

button {
  appearance: none;
}

input[type="number"] {
  appearance: textfield;
  -moz-appearance: textfield;
}

input[type="number"]::-webkit-inner-spin-button,
input[type="number"]::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

summary {
  display: flex;
  align-items: center;
  gap: var(--s2);
  list-style: none;
  line-height: 20px;
  border-radius: var(--r-sm);
  cursor: pointer;
}

summary::-webkit-details-marker {
  display: none;
}

summary::marker {
  content: '';
}

summary::after {
  content: '';
  flex: none;
  width: 7px;
  height: 7px;
  margin-left: auto;
  border-right: 1.5px solid currentColor;
  border-bottom: 1.5px solid currentColor;
  opacity: 0.7;
  transform: rotate(45deg);
  transition: transform 0.15s;
}

details[open] > summary::after {
  transform: rotate(-135deg);
}

summary:hover {
  color: var(--brand);
}

summary:focus-visible,
a:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 3px;
  border-radius: var(--r-sm);
}


input:focus,
select:focus {
  outline: none;
  border-color: var(--brand);
  box-shadow: var(--ring);
}

.table-scroll {
  margin: 0 calc(var(--s6) * -1);
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

th,
td {
  padding: 11px var(--s4);
  text-align: left;
  white-space: nowrap;
  border-bottom: 1px solid var(--line-soft);
  vertical-align: middle;
}

th:first-child,
td:first-child {
  padding-left: var(--s6);
}

th:last-child,
td:last-child {
  padding-right: var(--s6);
}

th {
  color: var(--muted);
  font-weight: 550;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.025em;
  background: var(--panel-2);
  border-bottom: 1px solid var(--line);
}


tbody tr:last-child td {
  border-bottom: none;
}

tbody tr:hover {
  background: var(--panel-2);
}


.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}


.mono {
  font-family: var(--mono);
  font-size: 12px;
}


.acts {
  display: flex;
  align-items: center;
  gap: var(--s1);
  justify-content: flex-end;
}

.wrapped {
  white-space: normal;
  min-width: 220px;
}


td.acts button {
  padding: 3px var(--s2);
  font-size: 12.5px;
}


.badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 9px 2px 8px;
  border-radius: var(--r-full);
  font-size: 12px;
  font-weight: 550;
  line-height: 1.6;
  white-space: nowrap;
  color: var(--muted-on-soft);
  background: var(--line-soft);
}

.badge::before {
  content: "";
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.badge.ok {
  color: var(--ok);
  background: var(--ok-bg);
}

.badge.warn {
  color: var(--warn);
  background: var(--warn-bg);
}

.badge.err {
  color: var(--err);
  background: var(--err-bg);
}

.badge.info {
  color: var(--brand);
  background: var(--info-bg);
}

.block {
  width: 100%;
}

.muted {
  color: var(--muted);
}

.small {
  font-size: 12.5px;
}

/* Preserve hierarchy when an action is unavailable, including primary actions. */
button:disabled {
  opacity: 0.58;
}

button.primary:disabled,
button.primary:disabled:hover {
  color: var(--on-brand);
  background: var(--brand);
  border-color: var(--brand);
}

button.danger:disabled:hover {
  background: var(--panel);
}

.center {
  text-align: center;
}

.err-text {
  color: var(--err);
  font-size: 12.5px;
}

a {
  color: var(--brand);
  text-decoration: none;
}

a:hover {
  text-decoration: underline;
}

input[type="date"]::-webkit-calendar-picker-indicator { filter: var(--date-icon-filter, none); }

@media (max-width: 720px) {
  .panel { padding: var(--s4); border-radius: var(--r); }
  .table-scroll { margin: 0 calc(var(--s4) * -1); }
  th:first-child, td:first-child { padding-left: var(--s4); }
  th:last-child, td:last-child { padding-right: var(--s4); }
  .field.sm, .field.grow { width: 100%; flex: 1 1 140px; min-width: 140px; }
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: 0.01ms !important; transition-duration: 0.01ms !important; }
}
`;

export const pageStyles = css`
:host { display: block; }

/* 侧边栏是 fixed 定位，不占文档流，由宿主让出这段宽度。 */
:host([admin]) {
  padding-left: var(--sidebar-w);
}

.wrap {
  min-width: 0;
  max-width: 1240px;
  padding: var(--s8);
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: var(--s5);
}

/* 可选宽度变体；后台页面在下方统一取消宽度上限。 */
.wrap.wide {
  max-width: 1600px;
}

.wrap.mid {
  max-width: 940px;
}

.wrap.narrow {
  max-width: 420px;
  padding: var(--s10) var(--s5);

  min-height: 100vh;
  justify-content: center;
}

/* 后台页面统一撑满侧边栏右侧，保留 .wrap 的响应式内边距。 */
:host([admin]) .wrap {
  max-width: none;
}


.page-head {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.page-head h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 650;
  letter-spacing: -0.02em;
  line-height: 1.3;
}

.page-head p {
  margin: 0;
  font-size: 13px;
  color: var(--muted);
  line-height: 1.65;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--s4);
}

.stat {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  padding: var(--s5) var(--s5) var(--s4);
  display: flex;
  flex-direction: column;
  gap: var(--s2);
  font-size: 12.5px;
  font-weight: 500;
  color: var(--muted);
}

.stat .k {
  letter-spacing: 0.01em;
}

.stat .v {
  font-size: 26px;
  font-weight: 650;
  line-height: 1.15;
  letter-spacing: -0.025em;
  color: var(--text);

  font-variant-numeric: tabular-nums;
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--s3);
  padding: var(--s10) var(--s5);
  color: var(--muted);
  font-size: 13px;
}

.empty::before {
  content: "";
  width: 28px;
  height: 28px;
  background: var(--muted-2);
  -webkit-mask: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'><path d='M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z'/></svg>") center / contain no-repeat;
  mask: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'><path d='M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z'/></svg>") center / contain no-repeat;
}

@media (max-width: 720px) {
  :host([admin]) { padding-left: 0; }
  .wrap { padding: var(--s4); gap: var(--s4); }
}
`;

export const modalStyles = css`
:host { display: contents; }

dialog.modal {

  border: 1px solid var(--line);
  padding: var(--s6);

  width: 100%;
  max-width: 400px;
  max-height: calc(100dvh - var(--s8));
  margin: auto;
  overflow-y: auto;
  overscroll-behavior: contain;

  box-sizing: border-box;

  background: var(--panel);
  border-radius: var(--r-lg);
  box-shadow: var(--shadow-lg);
  animation: pop 0.16s ease-out;
}


dialog.modal[open] {
  display: flex;
  flex-direction: column;
  gap: var(--s3);
}

dialog.modal::backdrop {
  background: light-dark(rgba(16, 20, 28, 0.42), rgba(0, 0, 0, 0.62));
  backdrop-filter: blur(2px);
  animation: fade 0.14s ease-out;
}


.modal h3 {
  font-size: 15px;
  font-weight: 650;
  color: var(--text);
  line-height: 1.45;
}

.modal .acts {
  margin-top: var(--s2);
}

@keyframes fade {
  from {
    opacity: 0;
  }
}

@keyframes pop {
  from {
    opacity: 0;
    transform: translateY(6px) scale(0.98);
  }
}

dialog.modal { color: var(--text); font: inherit; }

@media (max-width: 440px) { dialog.modal { width: calc(100% - var(--s8)); } }
`;
