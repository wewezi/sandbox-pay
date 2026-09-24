import { mustBe, api, setToken, fail, mountFloatingThemeToggle } from '/app.js';

/**
 * @typedef {import('./api-types.d.ts').LoginResult} LoginResult
 */

mountFloatingThemeToggle();

const form = mustBe('#form', HTMLFormElement);
const submit = mustBe('#submit', HTMLButtonElement);
const username = mustBe('#username', HTMLInputElement);
const password = mustBe('#password', HTMLInputElement);

form.addEventListener('submit', (e) => {
  e.preventDefault();
  void login();
});

async function login() {
  submit.disabled = true;
  submit.textContent = '登录中…';
  try {
    const data = /** @type {LoginResult} */ (
      await api.post('/auth/login', {
        username: username.value.trim(),
        password: password.value,
      })
    );
    setToken(data.token);
    location.replace(safeRedirect());
  } catch (err) {
    fail(err);
    submit.disabled = false;
    submit.textContent = '登录';
  }
}

/** 只接受站内相对路径，避免 ?redirect= 变成开放重定向。 */
function safeRedirect() {
  const raw = new URLSearchParams(location.search).get('redirect');
  if (!raw || !raw.startsWith('/') || raw.startsWith('//'))
    return '/orders.html';
  if (raw.startsWith('/login.html')) return '/orders.html';
  return raw;
}
