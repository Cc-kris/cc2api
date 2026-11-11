export const MOCK = 'http://127.0.0.1:18921'

async function post(path: string, body: unknown) {
  const r = await fetch(MOCK + path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })
  return r.json()
}

export async function scenario(key: string, sc: Record<string, unknown>) {
  return post('/__scenario', { key, scenario: sc })
}

export async function mockReset() {
  return post('/__reset', {})
}

export async function paymentConfig(cfg: Record<string, unknown>) {
  return post('/__payment/config', cfg)
}

export async function paymentNotify(outTradeNo: string) {
  return post('/__payment/notify', { out_trade_no: outTradeNo })
}

export async function releaseUpstream() {
  return post('/__release', {})
}

export async function mockRequests() {
  const r = await fetch(MOCK + '/__requests')
  const data = await r.json()
  return data.requests as any[]
}
