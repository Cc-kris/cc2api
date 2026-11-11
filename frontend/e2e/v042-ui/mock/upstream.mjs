// V03 v0.4.2 UI E2E mock upstream + payment gateway. Test-only.
import http from 'node:http'
import crypto from 'node:crypto'
import fs from 'node:fs'

const PORT = Number(process.env.MOCK_PORT || 18921)
const STATE_FILE = process.env.MOCK_STATE_FILE || '/tmp/v042-e2e-mock-state.json'
const PRICING_HASH_FILE = process.env.PRICING_HASH_FILE || ''

let state = {
  scenarios: {},
  requests: [],
  counters: {},
  payment: { pkey: 'v042e2epkey', pid: '1001', refundMode: 'success', queryMode: 'success', createMode: 'success', orders: {}, refunds: {} },
  releases: 0
}

function persist() { try { fs.writeFileSync(STATE_FILE, JSON.stringify(state, null, 2)) } catch { /* best-effort state snapshot */ } }
try {
  if (fs.existsSync(STATE_FILE)) {
    const saved = JSON.parse(fs.readFileSync(STATE_FILE, 'utf8'))
    state = { ...state, ...saved, payment: { ...state.payment, ...(saved.payment || {}) } }
  }
} catch { /* missing or corrupt state file: start from defaults */ }
function log(...a) { console.log(new Date().toISOString(), ...a) }
function readBody(req) { return new Promise((resolve) => { const cs = []; req.on('data', (c) => cs.push(c)); req.on('end', () => resolve(Buffer.concat(cs).toString('utf8'))) }) }
function sendJSON(res, status, obj, extra = {}) {
  const body = JSON.stringify(obj)
  res.writeHead(status, { 'content-type': 'application/json', 'content-length': Buffer.byteLength(body), ...extra })
  res.end(body)
}

// fail_first_n：同一个凭据的前 N 次请求使用 fail_mode（默认 message_start 前断流），
// 之后恢复正常，用于同账号重试/失败后成功的场景。
function pickScenario(key, sc) {
  if (!sc || sc.fail_first_n == null) return sc
  const seen = state.counters[key] || 0
  state.counters[key] = seen + 1
  persist()
  if (seen < sc.fail_first_n) return { ...sc, mode: sc.fail_mode || 'disconnect_before_start', ...(sc.fail_status ? { status: sc.fail_status } : {}) }
  return { ...sc, mode: sc.success_mode || 'normal', status: 200 }
}

function scenarioFor(req) {
  const creds = []
  for (const [k, v] of Object.entries(req.headers)) {
    if (!v) continue
    const val = Array.isArray(v) ? v.join(',') : String(v)
    creds.push(val)
    const m = val.match(/^Bearer\s+(.+)$/i)
    if (m) creds.push(m[1].trim())
    if (k === 'x-api-key' || k === 'x-goog-api-key' || k === 'api-key') creds.push(val.trim())
  }
  try { const u = new URL(req.url, 'http://x'); const qk = u.searchParams.get('key'); if (qk) creds.push(qk) } catch { /* unparsable URL: no query credential */ }
  for (const c of creds) { if (c && state.scenarios[c]) return { key: c, scenario: pickScenario(c, state.scenarios[c]) } }
  return { key: '*', scenario: pickScenario('*', state.scenarios['*'] || null) }
}
function findModel(body) { try { return JSON.parse(body).model || '' } catch { return '' } }
function commonHeaders(sc) { const h = { 'content-type': 'application/json' }; if (sc.request_id) h['x-request-id'] = sc.request_id; return h }

function anthropicUsage(sc) {
  const u = sc.usage || {}
  const usage = { input_tokens: u.input ?? 10, output_tokens: u.output ?? 5 }
  if (u.cache_read != null) usage.cache_read_input_tokens = u.cache_read
  if (u.cache_creation != null) usage.cache_creation_input_tokens = u.cache_creation
  if (u.cache_creation_5m != null) usage.cache_creation_5m_input_tokens = u.cache_creation_5m
  if (u.cache_creation_1h != null) usage.cache_creation_1h_input_tokens = u.cache_creation_1h
  if (u.cache_creation_5m != null || u.cache_creation_1h != null) {
    usage.cache_creation = {}
    if (u.cache_creation_5m != null) usage.cache_creation.ephemeral_5m_input_tokens = u.cache_creation_5m
    if (u.cache_creation_1h != null) usage.cache_creation.ephemeral_1h_input_tokens = u.cache_creation_1h
  }
  if (u.image_input != null) usage.input_tokens_details = { image_tokens: u.image_input }
  // 上游在用量里声明实际服务档位（AC24）；请求档位由客户端 body 决定。
  if (sc.speed) usage.speed = sc.speed
  return usage
}
function anthropicMessage(sc) {
  return { id: 'msg_' + crypto.randomUUID().replace(/-/g, '').slice(0, 20), type: 'message', role: 'assistant',
    model: sc.model || 'claude-sonnet-4-6', content: [{ type: 'text', text: sc.text || 'mock reply' }],
    stop_reason: 'end_turn', stop_sequence: null, usage: anthropicUsage(sc) }
}
function sse(res, event, data) { res.write('event: ' + event + '\ndata: ' + JSON.stringify(data) + '\n\n') }

function serveAnthropic(req, res, sc, body) {
  const mode = sc.mode || 'normal'
  if (mode === 'disconnect_before_start') { res.writeHead(200, commonHeaders(sc)); req.socket.destroy(); return }
  if (sc.status && sc.status >= 400) return sendJSON(res, sc.status, sc.error_body || { type: 'error', error: { type: 'api_error', message: 'mock upstream error' } }, commonHeaders(sc))
  const isStream = sc.stream !== false && /"stream"\s*:\s*true/.test(body)
  if (!isStream) return sendJSON(res, 200, anthropicMessage(sc), commonHeaders(sc))
  res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive', ...commonHeaders(sc) })
  const usage = anthropicUsage(sc)
  const inputUsage = { input_tokens: usage.input_tokens, output_tokens: 1 }
  for (const k of ['cache_read_input_tokens','cache_creation_input_tokens','cache_creation_5m_input_tokens','cache_creation_1h_input_tokens']) if (usage[k] != null) inputUsage[k] = usage[k]
  if (usage.input_tokens_details != null) inputUsage.input_tokens_details = usage.input_tokens_details
  if (sc.speed) inputUsage.speed = sc.speed
  sse(res, 'message_start', { type: 'message_start', message: { id: 'msg_' + crypto.randomUUID().replace(/-/g, '').slice(0, 20), type: 'message', role: 'assistant', model: sc.model || 'claude-sonnet-4-6', content: [], stop_reason: null, stop_sequence: null, usage: inputUsage } })
  sse(res, 'content_block_start', { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } })
  sse(res, 'content_block_delta', { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: sc.text || 'mock reply' } })
  sse(res, 'content_block_stop', { type: 'content_block_stop', index: 0 })
  const deltaUsage = { output_tokens: usage.output_tokens }
  for (const k of ['cache_read_input_tokens','cache_creation_input_tokens','cache_creation_5m_input_tokens','cache_creation_1h_input_tokens']) if (usage[k] != null) deltaUsage[k] = usage[k]
  if (usage.input_tokens_details != null) deltaUsage.input_tokens_details = usage.input_tokens_details
  if (sc.service_tier) deltaUsage.service_tier = sc.service_tier
  if (mode === 'disconnect_after_delta') {
    // 断在正文中间：已经输出内容，但还没有 stop_reason / message_stop，可换号续写。
    // usage 仍带上本次已观测输出，供续写耗尽时计费与尝试记录。
    sse(res, 'message_delta', { type: 'message_delta', delta: { stop_reason: null, stop_sequence: null }, usage: deltaUsage })
    setTimeout(() => { try { req.socket.destroy() } catch { /* socket already closed */ } }, 30)
    return
  }
  sse(res, 'message_delta', { type: 'message_delta', delta: { stop_reason: 'end_turn', stop_sequence: null }, usage: deltaUsage })
  if (mode === 'disconnect_after_usage') { setTimeout(() => { try { req.socket.destroy() } catch { /* socket already closed */ } }, 30); return }
  sse(res, 'message_stop', { type: 'message_stop' })
  res.end()
}

function openaiUsage(sc) {
  const u = sc.usage || {}
  const prompt = u.input ?? 10, completion = u.output ?? 5
  const usage = { prompt_tokens: prompt, completion_tokens: completion, total_tokens: prompt + completion }
  const details = {}
  if (u.cache_read != null) details.cached_tokens = u.cache_read
  if (u.image_input != null) details.image_tokens = u.image_input
  if (Object.keys(details).length) usage.prompt_tokens_details = details
  return usage
}
function serveOpenAI(req, res, sc, body) {
  const mode = sc.mode || 'normal'
  if (mode === 'disconnect_before_start') { res.writeHead(200, commonHeaders(sc)); req.socket.destroy(); return }
  if (sc.status && sc.status >= 400) return sendJSON(res, sc.status, sc.error_body || { error: { message: 'mock upstream error', type: 'api_error' } }, commonHeaders(sc))
  const isStream = sc.stream !== false && /"stream"\s*:\s*true/.test(body)
  const model = sc.model || 'gpt-5.4'
  if (!isStream) return sendJSON(res, 200, { id: 'chatcmpl-' + crypto.randomUUID().slice(0, 12), object: 'chat.completion', created: Math.floor(Date.now() / 1000), model, ...(sc.service_tier ? { service_tier: sc.service_tier } : {}), choices: [{ index: 0, message: { role: 'assistant', content: sc.text || 'mock reply' }, finish_reason: 'stop' }], usage: openaiUsage(sc) }, commonHeaders(sc))
  res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive', ...commonHeaders(sc) })
  const id = 'chatcmpl-' + crypto.randomUUID().slice(0, 12)
  const base = { id, object: 'chat.completion.chunk', created: Math.floor(Date.now() / 1000), model }
  const chunk = (delta) => 'data: ' + JSON.stringify({ ...base, choices: [{ index: 0, delta, finish_reason: null }] }) + '\n\n'
  res.write(chunk({ role: 'assistant', content: '' }))
  res.write(chunk({ content: sc.text || 'mock reply' }))
  res.write('data: ' + JSON.stringify({ ...base, choices: [{ index: 0, delta: {}, finish_reason: 'stop' }] }) + '\n\n')
  res.write('data: ' + JSON.stringify({ ...base, choices: [], usage: openaiUsage(sc), ...(sc.service_tier ? { service_tier: sc.service_tier } : {}) }) + '\n\n')
  if (mode === 'disconnect_after_usage') { setTimeout(() => { try { req.socket.destroy() } catch { /* socket already closed */ } }, 30); return }
  res.write('data: [DONE]\n\n'); res.end()
}
function serveOpenAIResponses(req, res, sc, body) {
  const mode = sc.mode || 'normal'
  if (mode === 'disconnect_before_start') { res.writeHead(200, commonHeaders(sc)); req.socket.destroy(); return }
  if (sc.status && sc.status >= 400) return sendJSON(res, sc.status, sc.error_body || { error: { message: 'mock upstream error' } }, commonHeaders(sc))
  const u = sc.usage || {}, input = u.input ?? 10, output = u.output ?? 5
  const response = { id: 'resp_' + crypto.randomUUID().replace(/-/g, '').slice(0, 20), object: 'response', created_at: Math.floor(Date.now() / 1000), status: 'completed', model: sc.model || 'gpt-5.4', ...(sc.service_tier ? { service_tier: sc.service_tier } : {}), output: [{ type: 'message', id: 'msg_1', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: sc.text || 'mock reply', annotations: [] }] }], usage: { input_tokens: input, output_tokens: output, total_tokens: input + output, input_tokens_details: { cached_tokens: u.cache_read || 0 } } }
  if (sc.stream !== false && /"stream"\s*:\s*true/.test(body)) {
    res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', ...commonHeaders(sc) })
    sse(res, 'response.output_text.delta', { type: 'response.output_text.delta', item_id: 'msg_1', output_index: 0, content_index: 0, delta: sc.text || 'mock reply' })
    sse(res, 'response.completed', { type: 'response.completed', response })
    res.write('data: [DONE]\n\n')
    return res.end()
  }
  return sendJSON(res, 200, response, commonHeaders(sc))
}
function serveGemini(req, res, sc) {
  const mode = sc.mode || 'normal'
  if (mode === 'disconnect_before_start') { res.writeHead(200, commonHeaders(sc)); req.socket.destroy(); return }
  if (sc.status && sc.status >= 400) return sendJSON(res, sc.status, sc.error_body || { error: { code: 500, message: 'mock upstream error', status: 'INTERNAL' } }, commonHeaders(sc))
  const u = sc.usage || {}, prompt = u.input ?? 10, out = u.output ?? 5
  const usageMetadata = { promptTokenCount: prompt, candidatesTokenCount: out, totalTokenCount: prompt + out }
  if (u.cache_read != null) usageMetadata.cachedContentTokenCount = u.cache_read
  const parts = sc.gemini_images ? [
    { inlineData: { mimeType: 'image/png', data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==' } },
    { inlineData: { mimeType: 'image/png', data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==' } },
    { fileData: { mimeType: 'image/png', fileUri: 'http://127.0.0.1:' + PORT + '/mock-images/three.png' } },
  ] : [{ text: sc.text || 'mock reply' }]
  return sendJSON(res, 200, { candidates: [{ content: { parts, role: 'model' }, finishReason: 'STOP', index: 0 }], usageMetadata, modelVersion: sc.model || 'gemini-2.5-pro' }, commonHeaders(sc))
}
function serveImages(req, res, sc) {
  if (sc.status && sc.status >= 400) return sendJSON(res, sc.status, sc.error_body || { error: { message: 'mock image error' } }, commonHeaders(sc))
  if (sc.images === 'unknown') return sendJSON(res, 200, { created: Math.floor(Date.now() / 1000) }, commonHeaders(sc))
  const n = sc.images ?? 1, data = []
  for (let i = 0; i < n; i++) data.push({ url: 'http://127.0.0.1:' + PORT + '/mock-images/' + i + '.png', revised_prompt: 'mock ' + i })
  return sendJSON(res, 200, { created: Math.floor(Date.now() / 1000), data }, commonHeaders(sc))
}

function easyPaySign(params, pkey) {
  const keys = Object.keys(params).filter((k) => k !== 'sign' && k !== 'sign_type' && params[k] !== '').sort()
  return crypto.createHash('md5').update(keys.map((k) => k + '=' + params[k]).join('&') + pkey).digest('hex')
}

async function servePayment(req, res, url, body) {
  const params = Object.fromEntries(new URLSearchParams(body))
  for (const [k, v] of url.searchParams) params[k] = v
  const p = state.payment
  // The tagged loopback transport preserves the official /authentication and /pa paths.
  // Also accept the earlier /api/v1-prefixed mock routes for EasyPay-style local fixtures.
  if (url.pathname.startsWith('/api/v1/')) url.pathname = url.pathname.slice('/api/v1'.length)
  if (url.pathname === '/authentication/login') {
    return sendJSON(res, 200, { token: 'v042-e2e-airwallex-token', expires_at: new Date(Date.now() + 3600000).toISOString() })
  }
  if (url.pathname === '/pa/payment_intents/create') {
    const order = JSON.parse(body || '{}')
    const intentID = 'pi_e2e_' + crypto.randomUUID().replace(/-/g, '').slice(0, 16)
    p.orders[order.merchant_order_id] = { trade_no: intentID, money: order.amount, status: 'SUCCEEDED' }
    persist()
    return sendJSON(res, 200, { id: intentID, client_secret: 'v042-e2e-client-secret', merchant_order_id: order.merchant_order_id, amount: order.amount, currency: order.currency, status: 'SUCCEEDED' })
  }
  if (url.pathname.startsWith('/pa/payment_intents/')) {
    const id = decodeURIComponent(url.pathname.slice('/pa/payment_intents/'.length))
    const order = Object.entries(p.orders).find(([, o]) => o.trade_no === id)
    if (!order) return sendJSON(res, 404, { code: 'not_found' })
    return sendJSON(res, 200, { id, merchant_order_id: order[0], amount: order[1].money, currency: 'CNY', status: 'SUCCEEDED', metadata: {} })
  }
  if (url.pathname === '/pa/refunds/create') {
    const order = JSON.parse(body || '{}')
    const rid = 're_e2e_' + crypto.randomUUID().replace(/-/g, '').slice(0, 16)
    p.refunds[order.request_id] = { refund_id: rid, status: 'RECEIVED' }
    persist()
    return sendJSON(res, 200, { id: rid, request_id: order.request_id, payment_intent_id: order.payment_intent_id, amount: order.amount, currency: 'CNY', status: 'RECEIVED' })
  }
  if (url.pathname.startsWith('/pa/refunds/')) {
    const id = decodeURIComponent(url.pathname.slice('/pa/refunds/'.length))
    const refund = Object.values(p.refunds).find((r) => r.refund_id === id)
    if (!refund) return sendJSON(res, 404, { code: 'not_found' })
    return sendJSON(res, 200, { id, status: p.queryMode === 'fail' ? 'FAILED' : 'SETTLED' })
  }
  if (url.pathname === '/mapi.php' || url.pathname === '/submit.php') {
    const outTradeNo = params.out_trade_no || ''
    const tradeNo = 'MOCK' + crypto.randomUUID().replace(/-/g, '').slice(0, 16).toUpperCase()
    if (p.createMode === 'fail') return sendJSON(res, 200, { code: -1, msg: 'mock create failed' })
    p.orders[outTradeNo] = { trade_no: tradeNo, money: params.money, notify_url: params.notify_url, return_url: params.return_url, notified: false, created_at: Date.now() }
    persist()
    if (url.pathname === '/submit.php') { res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }); return res.end('<html><body>mock easypay checkout ' + outTradeNo + '</body></html>') }
    return sendJSON(res, 200, { code: 1, msg: 'success', trade_no: tradeNo, payurl: 'http://127.0.0.1:' + PORT + '/pay/' + tradeNo, qrcode: 'mock://pay/' + tradeNo })
  }
  if (url.pathname === '/api.php' && params.act === 'order') {
    const o = p.orders[params.out_trade_no]
    if (!o) return sendJSON(res, 200, { code: -1, msg: 'order not found' })
    return sendJSON(res, 200, { code: 1, msg: 'success', status: 1, money: o.money, trade_no: o.trade_no })
  }
  if (url.pathname === '/api.php' && params.act === 'refund') {
    if (p.refundMode === 'fail') return sendJSON(res, 500, { code: -1, msg: 'mock refund failed' })
    const rid = 'RF' + crypto.randomUUID().replace(/-/g, '').slice(0, 12)
    p.refunds[params.out_trade_no || params.trade_no || 'x'] = { refund_id: rid, status: 'success' }
    persist()
    return sendJSON(res, 200, { code: 1, msg: 'success', refund_no: rid })
  }
  sendJSON(res, 404, { code: -1, msg: 'not found' })
}

async function control(res, url, body) {
  const p = state.payment
  if (url.pathname === '/__scenario') { const { key, scenario } = JSON.parse(body || '{}'); state.scenarios[key || '*'] = scenario; persist(); return sendJSON(res, 200, { ok: true }) }
  if (url.pathname === '/__reset') { state.scenarios = {}; state.requests = []; state.counters = {}; state.releases = 0; p.orders = {}; p.refunds = {}; p.refundMode = 'success'; p.queryMode = 'success'; p.createMode = 'success'; persist(); return sendJSON(res, 200, { ok: true }) }
  if (url.pathname === '/__release') { state.releases += 1; persist(); return sendJSON(res, 200, { ok: true, releases: state.releases }) }
  if (url.pathname === '/__requests') return sendJSON(res, 200, { requests: state.requests })
  if (url.pathname === '/__payment/config') { Object.assign(p, JSON.parse(body || '{}')); persist(); return sendJSON(res, 200, { ok: true }) }
  if (url.pathname === '/__payment/notify-airwallex') {
    const { out_trade_no } = JSON.parse(body || '{}')
    const o = p.orders[out_trade_no]
    if (!o) return sendJSON(res, 404, { ok: false, error: 'order not found' })
    const payload = JSON.stringify({ id: 'evt_e2e_' + Date.now(), name: 'payment_intent.succeeded', data: { object: { id: o.trade_no, merchant_order_id: out_trade_no, amount: o.money, currency: 'CNY', status: 'SUCCEEDED', metadata: {} } } })
    const timestamp = String(Date.now())
    const signature = crypto.createHmac('sha256', p.webhookSecret || 'v042-e2e-airwallex-webhook').update(timestamp + payload).digest('hex')
    try {
      const r = await fetch('http://127.0.0.1:8280/api/v1/payment/webhook/airwallex', { method: 'POST', headers: { 'content-type': 'application/json', 'x-timestamp': timestamp, 'x-signature': signature }, body: payload })
      return sendJSON(res, 200, { ok: r.ok, status: r.status })
    } catch (e) { return sendJSON(res, 502, { ok: false, error: String(e) }) }
  }
  if (url.pathname === '/__payment/orders') return sendJSON(res, 200, { orders: p.orders, refunds: p.refunds })
  if (url.pathname === '/__payment/notify') {
    const { out_trade_no } = JSON.parse(body || '{}')
    const o = p.orders[out_trade_no]
    if (!o) return sendJSON(res, 404, { ok: false, error: 'order not found' })
    const notify = { pid: p.pid, trade_no: o.trade_no, out_trade_no, type: 'alipay', name: 'mock recharge', money: o.money, trade_status: 'TRADE_SUCCESS' }
    notify.sign = easyPaySign(notify, p.pkey)
    notify.sign_type = 'MD5'
    if (!o.notify_url) return sendJSON(res, 400, { ok: false, error: 'no notify_url' })
    try {
      const r = await fetch(o.notify_url, { method: 'POST', headers: { 'content-type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams(notify).toString() })
      const text = await r.text()
      o.notified = r.ok; persist()
      return sendJSON(res, 200, { ok: r.ok, status: r.status, body: text })
    } catch (e) { return sendJSON(res, 502, { ok: false, error: String(e) }) }
  }
  return sendJSON(res, 404, { ok: false })
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, 'http://x')
  const body = await readBody(req)
  if (url.pathname.startsWith('/__')) return control(res, url, body)
  // Keep the backend's periodic price synchronization observable and loopback-only.
  if (url.pathname === '/unused-pricing.sha256') {
    state.requests.push({ ts: Date.now(), method: req.method, path: url.pathname, key: '*', model: '' })
    persist()
    if (!PRICING_HASH_FILE) return sendJSON(res, 503, { error: 'pricing hash fixture is not configured' })
    try {
      const body = fs.readFileSync(PRICING_HASH_FILE, 'utf8').trim() + '\n'
      res.writeHead(200, { 'content-type': 'text/plain', 'content-length': Buffer.byteLength(body) })
      return res.end(body)
    } catch { return sendJSON(res, 503, { error: 'pricing hash fixture is unavailable' }) }
  }
  // Airwallex uses an explicitly compiled loopback-only transport in the E2E binary.
  // Production domain validation and provider business logic are unchanged.
  if (url.pathname.startsWith('/api/v1/') || url.pathname === '/authentication/login' || url.pathname.startsWith('/pa/')) return servePayment(req, res, url, body)
  if (['/mapi.php', '/submit.php', '/api.php'].includes(url.pathname)) return servePayment(req, res, url, body)
  const { key, scenario } = scenarioFor(req)
  state.requests.push({ ts: Date.now(), method: req.method, path: url.pathname + url.search, key, model: findModel(body) })
  persist()
  if (state.requests.length > 500) state.requests.splice(0, state.requests.length - 500)
  const sc = scenario || {}
  if (sc.hold) { const target = state.releases + 1; while (state.releases < target) await new Promise((r) => setTimeout(r, 50)) }
  if (sc.delay_ms) await new Promise((r) => setTimeout(r, sc.delay_ms))
  if (url.pathname.endsWith('/v1/messages') || url.pathname.endsWith('/v1/messages/')) return serveAnthropic(req, res, sc, body)
  if (url.pathname.endsWith('/v1/chat/completions')) return serveOpenAI(req, res, sc, body)
  if (url.pathname.endsWith('/v1/responses')) return serveOpenAIResponses(req, res, sc, body)
  if (url.pathname.endsWith('/images/generations') || url.pathname.endsWith('/images/edits')) return serveImages(req, res, sc)
  if (url.pathname.includes(':generateContent') || url.pathname.includes(':streamGenerateContent') || url.pathname.includes('/v1beta/models/')) return serveGemini(req, res, sc)
  if (url.pathname.startsWith('/mock-images/')) { const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64'); res.writeHead(200, { 'content-type': 'image/png', 'content-length': png.length }); return res.end(png) }
  log('unhandled', req.method, url.pathname)
  sendJSON(res, 404, { error: 'mock unhandled ' + url.pathname })
})
server.listen(PORT, '127.0.0.1', () => log('v042 e2e mock listening on ' + PORT))
