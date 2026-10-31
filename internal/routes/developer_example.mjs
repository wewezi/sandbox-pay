// Mintweft Node.js demo, Node.js 22+; no external dependencies. Configure secrets through environment variables.
// Native paymentUrl opens /cashier?orderNo=...; the frontend entry points are /sandbox and /developer.
import { createServer } from 'node:http';
import { createHmac, timingSafeEqual, createHash } from 'node:crypto';
import { readFile, writeFile, rename } from 'node:fs/promises';

const base = (process.env.SANDBOX_PAY_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
const token = process.env.SANDBOX_PAY_API_TOKEN;
const secret = process.env.SANDBOX_PAY_WEBHOOK_SECRET;
const port = Number(process.env.DEMO_PORT || 3309);
const stateFile = process.env.DEMO_STATE_FILE || './sandbox-demo-events.json';
if (!token || !secret) throw new Error('Set SANDBOX_PAY_API_TOKEN and SANDBOX_PAY_WEBHOOK_SECRET before starting.');
if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('DEMO_PORT must be 1..65535.');
let state = { events: [], orders: {} };
try { state = JSON.parse(await readFile(stateFile, 'utf8')); }
catch (error) { if (error.code !== 'ENOENT') throw error; }
let transaction = Promise.resolve();
async function bodyOf(request) {
  const chunks = []; let length = 0;
  for await (const chunk of request) { length += chunk.length; if (length > 65536) throw new Error('Request body too large.'); chunks.push(chunk); }
  return Buffer.concat(chunks);
}
async function gateway(path, method = 'GET', body) {
  const response = await fetch(base + '/v1/api/gateway' + path, {
    method, headers: { Authorization: 'Bearer ' + token, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined, signal: AbortSignal.timeout(10000), redirect: 'error',
  });
  const result = await response.json();
  if (!response.ok || result.code !== 200) throw new Error(result.msg || 'Gateway request failed.');
  return result.data;
}
function respond(response, status, data) { response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' }); response.end(JSON.stringify(data)); }
const server = createServer(async (request, response) => {
  try {
    const path = new URL(request.url, 'http://localhost').pathname;
    if (request.method === 'GET' && path === '/') return respond(response, 200, { instructions: 'POST /orders with amount, productName and externalOrderNo, then open the returned paymentUrl (/cashier?orderNo=...); signed callbacks arrive at POST /notify.' });
    if (request.method === 'POST' && path === '/orders') {
      const input = JSON.parse((await bodyOf(request)).toString('utf8'));
      if (!input.externalOrderNo) return respond(response, 400, { error: 'externalOrderNo is required for retry-safe orders.' });
      return respond(response, 200, await gateway('/pay', 'POST', {
        amount: input.amount, productName: input.productName || 'Demo order', externalOrderNo: input.externalOrderNo,
        payMethod: 'native', notifyUrl: process.env.DEMO_NOTIFY_URL || `http://127.0.0.1:${port}/notify`,
      }));
    }
    if (request.method === 'POST' && path === '/notify') {
      const raw = await bodyOf(request);
      const timestamp = String(request.headers['x-sandbox-pay-timestamp'] || '');
      const nonce = String(request.headers['x-sandbox-pay-nonce'] || '');
      const signature = String(request.headers['x-sandbox-pay-signature'] || '');
      // This demo creates new orders signed with the independent project secret (v2).
      const version = String(request.headers['x-sandbox-pay-signature-version'] || '');
      if (version !== '2' || !timestamp || !nonce || !/^[a-f0-9]{64}$/i.test(signature)) return respond(response, 401, { error: 'Invalid callback authentication.' });
      // Preserve the original JSON bytes. Signatures persist across delayed retries.
      const expected = createHmac('sha256', secret).update(Buffer.concat([raw, Buffer.from('&timestamp=' + timestamp + '&nonce=' + nonce)])).digest();
      if (!timingSafeEqual(expected, Buffer.from(signature, 'hex'))) return respond(response, 401, { error: 'Invalid callback authentication.' });
      const event = JSON.parse(raw.toString('utf8'));
      if (!event.orderNo || !Number.isFinite(event.amount)) return respond(response, 400, { error: 'Malformed callback.' });
      const eventId = event.eventId || createHash('sha256').update(raw).update(timestamp).update(nonce).digest('hex');
      const work = async () => {
        if (state.events.includes(eventId)) return;
        const order = await gateway('/query?orderNo=' + encodeURIComponent(event.orderNo));
        if (order.callbackSignatureVersion !== 2) throw new Error('This demo requires a v2 project-secret order.');
        if (order.externalOrderNo !== event.externalOrderNo || order.amount !== event.amount) throw new Error('Callback does not match the project order.');
        const next = { events: [...state.events, eventId], orders: { ...state.orders, [order.externalOrderNo || order.orderNo]: order } };
        // Persist before acknowledging; retries of the same event perform no business update.
        await writeFile(stateFile + '.tmp', JSON.stringify(next, null, 2), { mode: 0o600 });
        await rename(stateFile + '.tmp', stateFile); state = next;
      };
      transaction = transaction.catch(() => {}).then(work);
      await transaction;
      return respond(response, 200, { received: true });
    }
    respond(response, 404, { error: 'Not found.' });
  } catch { respond(response, 500, { error: 'Demo request failed; retry after checking configuration.' }); }
});
server.listen(port, '127.0.0.1', () => process.stdout.write(`Mintweft demo listening at http://127.0.0.1:${port}\n`));
process.on('SIGINT', () => server.close());
process.on('SIGTERM', () => server.close());
