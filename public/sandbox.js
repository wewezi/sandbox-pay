import {
  mustBe,
  api,
  ok,
  fail,
  requireAuth,
  renderTopbar,
  confirmDialog,
} from '/app.js';

/**
 * @typedef {import('./api-types.d.ts').GatewayPayResult} GatewayPayResult
 */

if (requireAuth()) {
  renderTopbar('/sandbox.html');
  bindTestPay();
  bindReset();
}

function bindTestPay() {
  const form = mustBe('#testPay', HTMLFormElement);
  const submit = mustBe('button[type=submit]', HTMLButtonElement, form);
  const amount = mustBe('#amount', HTMLInputElement);
  const productName = mustBe('#productName', HTMLInputElement);

  form.addEventListener('submit', (e) => {
    e.preventDefault();
    void pay();
  });

  async function pay() {
    submit.disabled = true;
    try {
      const res = /** @type {GatewayPayResult} */ (
        await api.post('/admin/test-pay', {
          amount: Number(amount.value),
          productName: productName.value.trim() || undefined,
        })
      );
      if (!res?.data) throw new Error('后端未返回收银台地址');
      location.href = res.data;
    } catch (err) {
      fail(err);
      submit.disabled = false;
    }
  }
}

function bindReset() {
  mustBe('#reset', HTMLButtonElement).addEventListener(
    'click',
    () => void reset(),
  );
}

async function reset() {
  const yes = await confirmDialog({
    title: '清空订单与通知',
    message:
      '所有订单和通知记录都会被删除，不可恢复。分组、Key 和管理员账号保留。',
    confirmText: '确认清空',
    danger: true,
  });
  if (!yes) return;
  try {
    await api.post('/admin/reset-data');
    ok('已清空');
  } catch (err) {
    fail(err);
  }
}
