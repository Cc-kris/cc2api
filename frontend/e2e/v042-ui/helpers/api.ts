export const API = 'http://127.0.0.1:8280/api/v1'
export const GATEWAY = 'http://127.0.0.1:8280'
export const MOCK = 'http://127.0.0.1:18921'
export const ADMIN_EMAIL = 'admin@v042e2e.local'
export const ADMIN_PASSWORD = 'V042-e2e-Admin-Passw0rd!'

export async function raw(method: string, url: string, body?: any, token?: string, headers: Record<string, string> = {}) {
  const res = await fetch(url, {
    method,
    headers: { 'content-type': 'application/json', ...(token ? { authorization: 'Bearer ' + token } : {}), ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await res.text()
  let json = null
  try { json = JSON.parse(text) } catch {}
  return { status: res.status, text, json }
}

export async function api(method: string, path: string, body?: any, token?: string): Promise<any> {
  const r = await raw(method, API + path, body, token)
  if (r.status >= 300 || (r.json && r.json.code !== undefined && r.json.code !== 0)) {
    throw new Error(method + ' ' + path + ' -> ' + r.status + ' ' + r.text.slice(0, 300))
  }
  return r.json ? r.json.data : undefined
}

async function loginWithRealRateLimit(email: string, password: string) {
  // Production auth intentionally permits 20 logins per IP per minute. The
  // serial two-viewport suite exceeds that; respect the real 429 window rather
  // than disabling a security control or bypassing the actual auth endpoint.
  for (let attempt = 0; attempt < 2; attempt++) {
    const r = await raw('POST', API + '/auth/login', { email, password })
    if (r.status === 429) {
      await new Promise(resolve => setTimeout(resolve, 61000))
      continue
    }
    if (r.status >= 300 || (r.json && r.json.code !== undefined && r.json.code !== 0)) {
      throw new Error('POST /auth/login -> ' + r.status + ' ' + r.text.slice(0, 300))
    }
    return r.json.data.access_token
  }
  throw new Error('POST /auth/login -> authentication rate limit did not reset')
}
export async function adminLogin() {
  return loginWithRealRateLimit(ADMIN_EMAIL, ADMIN_PASSWORD)
}
export async function userLogin(email: string, password: string) {
  return loginWithRealRateLimit(email, password)
}

let seq = 0
export function uniq(prefix: string) {
  seq += 1
  return prefix + '-' + Date.now().toString(36) + '-' + seq
}

export async function createGroup(token: string, body: any) { return api('POST', '/admin/groups', body, token) }
export async function updateGroup(token: string, id: number, body: any) { return api('PUT', '/admin/groups/' + id, body, token) }
export async function createAccount(token: string, body: any) { return api('POST', '/admin/accounts', body, token) }
export async function updateAccount(token: string, id: number, body: any) { return api('PUT', '/admin/accounts/' + id, body, token) }
export async function createUser(token: string, body: any) { return api('POST', '/admin/users', body, token) }
export async function setBalance(token: string, userId: number, balance: string | number, operation = 'set') { return api('POST', '/admin/users/' + userId + '/balance', { balance, operation }, token) }
export async function createKey(token: string, body: any) { return api('POST', '/keys', body, token) }
export async function getUser(token: string, id: number) { return api('GET', '/admin/users/' + id, undefined, token) }
export async function listUsage(token: string, params: Record<string, string> = {}) {
  const q = new URLSearchParams(params).toString()
  return api('GET', '/admin/usage' + (q ? '?' + q : ''), undefined, token)
}
export async function createChannel(token: string, body: any) { return api('POST', '/admin/channels', body, token) }
export async function updateChannel(token: string, id: number, body: any) { return api('PUT', '/admin/channels/' + id, body, token) }
export async function getAdminUsage(token: string, params: Record<string, string> = {}) {
  const q = new URLSearchParams(params).toString()
  return api('GET', '/admin/usage' + (q ? '?' + q : ''), undefined, token)
}
export async function listAccounts(token: string, params: Record<string, string> = {}) {
  const q = new URLSearchParams(params).toString()
  return api('GET', '/admin/accounts' + (q ? '?' + q : ''), undefined, token)
}

interface GatewayRequest { path: string; apiKey: string; body: any; headers?: Record<string, string> }

/** Gateway request that tolerates upstream stream aborts (fetch may reject). */
export async function gatewayTry(req: GatewayRequest) {
  try {
    const r = await gateway(req)
    return { ...r, threw: false }
  } catch (e) {
    return { status: 0, text: String(e), json: null, threw: true }
  }
}
export async function gatewayOpenAITry(req: GatewayRequest) {
  try {
    const r = await gatewayOpenAI(req)
    return { ...r, threw: false }
  } catch (e) {
    return { status: 0, text: String(e), json: null, threw: true }
  }
}

export function openaiChatBody(model: string, opts: Record<string, any> = {}) {
  return { model, messages: [{ role: 'user', content: 'hello' }], ...opts }
}
export function openaiResponsesBody(model: string, opts: Record<string, any> = {}) {
  return { model, input: 'hello', ...opts }
}
export function imageBody(model: string, opts: Record<string, any> = {}) {
  return { model, prompt: 'a cat', n: 3, ...opts }
}

export async function gateway(req: GatewayRequest) {
  return raw('POST', GATEWAY + req.path, req.body, undefined, { 'x-api-key': req.apiKey, 'anthropic-version': '2023-06-01', ...(req.headers || {}) })
}
export async function gatewayOpenAI(req: GatewayRequest) {
  return raw('POST', GATEWAY + req.path, req.body, undefined, { authorization: 'Bearer ' + req.apiKey, ...(req.headers || {}) })
}
export function anthropicBody(model: string, stream = false, maxTokens = 64) {
  return { model, max_tokens: maxTokens, stream, messages: [{ role: 'user', content: 'hello' }] }
}
