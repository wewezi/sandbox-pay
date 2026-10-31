import { css } from 'lit';

export const loginStyles = css`
  :host {
    --login-button: light-dark(#047a40, #34d17a);
    --login-button-hover: light-dark(#036837, #5adf96);
    display: block;
    min-height: 100svh;
    background: var(--bg);
  }

  .login-page { min-height: 100svh; position: relative; }
  .login-main { width: 100%; max-width: 1056px; min-height: 100svh; margin: 0 auto; display: grid; grid-template-columns: minmax(0, 1fr) 360px; align-items: center; gap: clamp(64px, 8vw, 120px); padding: 96px 48px; }
  .login-art { display: grid; place-items: center; min-width: 0; }
  .dot-logo { display: block; width: 400px; max-width: 100%; aspect-ratio: 1; background: var(--brand); -webkit-mask: url('/logo-dots.svg') center / contain no-repeat; mask: url('/logo-dots.svg') center / contain no-repeat; }
  .login-card { width: 360px; max-width: 100%; }

  .brand-lockup { margin-bottom: 38px; }
  .brand-lockup h1 { margin: 0; font-size: 24px; line-height: 1.25; font-weight: 650; letter-spacing: -0.04em; }

  .theme-slot { position: absolute; top: 24px; right: 24px; min-width: 44px; min-height: 44px; display: grid; place-items: center; }
  form { display: flex; flex-direction: column; gap: 22px; }
  .login-field { display: flex; flex-direction: column; gap: 10px; }
  label { color: var(--text); font-size: 13px; font-weight: 500; }

  .input-shell { display: flex; align-items: center; min-height: 50px; border: 1px solid var(--line); border-radius: 9px; background: var(--panel); transition: border-color 0.15s, box-shadow 0.15s; }
  .input-shell:hover { border-color: var(--muted-2); }
  .input-shell:focus-within { border-color: var(--brand); box-shadow: var(--ring); }
  input { min-width: 0; min-height: 48px; padding: 12px 14px; color: var(--text); font-size: 14px; line-height: 24px; border: 0; border-radius: 9px; background: transparent; box-shadow: none; }
  input:focus { box-shadow: none; }
  input:disabled { cursor: wait; }

  .password-toggle { width: 44px; min-width: 44px; height: 44px; padding: 0; margin-right: 3px; border: 0; border-radius: 7px; background: transparent; color: var(--muted); }
  .password-toggle:hover { color: var(--text); background: var(--line-soft); }
  .password-toggle svg { width: 19px; height: 19px; }
  .password-toggle:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; box-shadow: none; }
  .input-shell.invalid { border-color: var(--err); }
  .login-error { padding: 11px 13px; background: var(--err-bg); color: var(--err); border-radius: 9px; font-size: 12px; line-height: 1.7; }

  .login-submit { width: 100%; min-height: 50px; margin-top: 5px; padding: 12px 18px; gap: 10px; border: 1px solid var(--login-button); border-radius: 9px; background: var(--login-button); color: light-dark(#fff, #082015); font-size: 14px; font-weight: 600; }
  .login-submit:hover { background: var(--login-button-hover); border-color: var(--login-button-hover); }
  .login-submit:disabled { background: var(--login-button); color: light-dark(#fff, #082015); opacity: 0.65; cursor: wait; }
  .spinner { width: 16px; height: 16px; border: 2px solid currentColor; border-right-color: transparent; border-radius: 50%; animation: spin 0.7s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  @media (max-width: 800px) {
    .login-main { display: flex; flex-direction: column; justify-content: center; gap: 28px; padding: 88px 28px 56px; }
    .dot-logo { width: 128px; }
    .brand-lockup { margin-bottom: 30px; }
  }

  @media (max-width: 440px) {
    .login-main { gap: 24px; }
    .dot-logo { width: 112px; }
    .brand-lockup { margin-bottom: 28px; }
    .brand-lockup h1 { font-size: 22px; }
    .theme-slot { top: 16px; right: 16px; }
    input { font-size: 16px; }
  }

  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after { animation: none !important; transition: none !important; }
  }
`;
