import { css } from 'lit';

export const navigationStyles = css`
:host { display: block; }

.topbar {
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  width: var(--sidebar-w);
  display: flex;
  flex-direction: column;
  gap: var(--s1);
  padding: var(--s5) var(--s3);
  background: var(--panel);
  border-right: 1px solid var(--line);
  z-index: 20;
}

.brand {
  display: flex;
  align-items: center;
  gap: var(--s2);
  padding: 0 var(--s2) var(--s5);
  font-size: 15px;
  font-weight: 650;
  letter-spacing: -0.02em;
  color: var(--text);
}


.brand::before {
  content: "";
  width: 28px;
  height: 28px;
  flex: none;
  background: var(--brand);
  -webkit-mask: url('/favicon.svg') center / contain no-repeat;
  mask: url('/favicon.svg') center / contain no-repeat;
}

.topbar nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-height: 0;
  overflow-y: auto;
}

.topbar nav a {
  display: flex;
  align-items: center;
  gap: var(--s3);
  padding: var(--s2) var(--s2);
  border-radius: var(--r-sm);
  color: var(--muted);
  font-size: 13.5px;
  font-weight: 500;
  text-decoration: none;
  transition: background-color 0.12s, color 0.12s;
}

.topbar nav a:hover {
  background: var(--line-soft);
  color: var(--text);
}

.topbar nav a.active {
  background: var(--brand-soft);
  color: var(--brand);
  font-weight: 600;
}

.spacer {
  flex: 1;

  border-bottom: 1px solid var(--line-soft);
  margin: var(--s3) var(--s2) var(--s2);
}


.topbar nav a::before {
  content: "";
  width: 17px;
  height: 17px;
  flex: none;
  background: currentColor;
  -webkit-mask: var(--icon) center / contain no-repeat;
  mask: var(--icon) center / contain no-repeat;
}

.topbar nav a[href="/dashboard"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><rect x='3' y='3' width='7' height='7' rx='1'/><rect x='14' y='3' width='7' height='7' rx='1'/><rect x='3' y='14' width='7' height='7' rx='1'/><path d='M14 21v-4m4 4V13m4 8v-6'/></svg>");
}

.topbar nav a[href="/orders"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round'><path d='M9 6h11M9 12h11M9 18h11M4.5 6h.01M4.5 12h.01M4.5 18h.01'/></svg>");
}

.topbar nav a[href="/notifications"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><path d='M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z'/><path d='M14 2v6h6M8 13h8M8 17h6'/></svg>");
}

.topbar nav a[href="/sandbox"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><path d='M9 3h6v6l4 9a2 2 0 0 1-1.8 3H6.8A2 2 0 0 1 5 18l4-9V3'/><path d='M8 3h8'/></svg>");
}


.topbar nav a[href="/groups"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><circle cx='7.5' cy='15.5' r='4.5'/><path d='M11 12l9-9M16 3l4 4M15 8l2 2'/></svg>");
}

.topbar nav a[href="/developer"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><path d='m8 7-5 5 5 5m8-10 5 5-5 5m-3-13-2 16'/></svg>");
}
.topbar nav a[href="/operations"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><path d='M12 3v3m0 12v3M3 12h3m12 0h3M5.6 5.6l2.1 2.1m8.6 8.6 2.1 2.1M5.6 18.4l2.1-2.1m8.6-8.6 2.1-2.1'/><circle cx='12' cy='12' r='4'/></svg>");
}
.topbar nav a[href="/team"] {
  --icon: url("data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'><circle cx='9' cy='8' r='3'/><path d='M3 21v-3a6 6 0 0 1 12 0v3m1-18a3 3 0 0 1 0 6m2 5a5 5 0 0 1 3 4v3'/></svg>");
}

sp-theme-toggle { --theme-toggle-width: 100%; }
@media (max-width: 720px) {
  :host { position: sticky; top: 0; z-index: 20; }
  .topbar { position: static; inset: auto; width: auto; flex-direction: row; align-items: center;
    gap: var(--s2); padding: calc(var(--s2) + env(safe-area-inset-top, 0px)) max(var(--s3), env(safe-area-inset-right, 0px)) var(--s2) max(var(--s3), env(safe-area-inset-left, 0px));
    border-right: none; border-bottom: 1px solid var(--line); }
  .brand { padding: 0; font-size: 0; gap: 0; }
  .topbar nav {
    position: fixed;
    inset: auto max(var(--s3), env(safe-area-inset-right, 0px)) calc(var(--s3) + env(safe-area-inset-bottom, 0px)) max(var(--s3), env(safe-area-inset-left, 0px));
    width: max-content;
    max-width: calc(100% - max(var(--s3), env(safe-area-inset-left, 0px)) - max(var(--s3), env(safe-area-inset-right, 0px)));
    margin-inline: auto;
    flex-direction: row;
    align-items: center;
    gap: 2px;
    padding: 6px;
    overflow: visible;
    background: color-mix(in srgb, var(--panel) 92%, transparent);
    border: 1px solid var(--line);
    border-radius: var(--r-full);
    box-shadow: var(--shadow-lg);
    -webkit-backdrop-filter: blur(16px);
    backdrop-filter: blur(16px);
  }
  .topbar nav a {
    flex: 1 1 44px;
    justify-content: center;
    width: 44px;
    min-width: 0;
    height: 44px;
    gap: 0;
    padding: 0;
    border-radius: var(--r-full);
  }
  .topbar nav a::before { width: 20px; height: 20px; }
  .topbar nav a span { display: none; }
  .topbar nav a:focus-visible { outline-offset: 1px; border-radius: var(--r-full); }
  .topbar > sp-theme-toggle, .topbar > button { flex: none; }
  .spacer { border: 0; margin: 0; }
}
`;

export const themeStyles = css`
:host { display: block; }

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

.theme-toggle { width: var(--theme-toggle-width, auto); }

@media (max-width: 720px) { .theme-label { display: none; } }
`;

export const pagerStyles = css`
:host { display: block; }
.pager {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s2);
  padding-top: var(--s3);
  border-top: 1px solid var(--line-soft);
}

.pager .info {
  min-width: 0;
  margin-right: auto;
  font-size: 12.5px;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
  line-height: 1.6;
}

@media (max-width: 480px) {
  .pager { justify-content: flex-end; row-gap: var(--s3); }
  .pager .info { width: 100%; margin-right: 0; }
  .pager button { min-height: 38px; }
}
`;

export const toastStyles = css`
:host {
  position: fixed;
  top: var(--s4);
  right: var(--s4);
  z-index: 200;
  display: flex;
  flex-direction: column;
  gap: var(--s2);
  pointer-events: none;
}

.toast {
  max-width: 340px;
  padding: 11px var(--s4);
  font-size: 13px;
  line-height: 1.5;
  color: var(--text);
  background: var(--panel);
  border: 1px solid var(--line);
  border-left: 3px solid var(--muted-2);
  border-radius: var(--r-sm);
  box-shadow: var(--shadow-lg);
  animation: slide 0.18s ease-out;
}

.toast.ok {
  border-left-color: var(--ok);
}

.toast.err {
  border-left-color: var(--err);
}
@keyframes slide {
  from {
    opacity: 0;
    transform: translateX(12px);
  }
}
@media (max-width: 400px) { :host { left: var(--s4); } .toast { max-width: 100%; } }
`;
