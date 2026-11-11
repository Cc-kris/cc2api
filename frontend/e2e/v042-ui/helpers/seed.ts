import {
  adminLogin,
  createAccount,
  createChannel,
  createGroup,
  createKey,
  createUser,
  uniq,
  userLogin,
} from './api'

export const MOCK_UPSTREAM = 'http://127.0.0.1:18921'

export interface SeedAccount {
  name?: string
  platform?: string
  type?: string
  credentials?: Record<string, unknown>
  priority?: number
  concurrency?: number
  extra?: Record<string, unknown>
}

export interface SeedOptions {
  prefix: string
  platform?: string
  rate?: number
  balance?: number
  quota?: number
  group?: Record<string, unknown>
  accounts?: SeedAccount[]
  channel?: { name?: string; model_pricing: unknown[] }
}

/**
 * Seed a group, one or more accounts, a user with balance and an API key using
 * only the real admin/user API. Every spec calls this after global setup has
 * truncated the E2E database, so the suite is repeatable.
 */
export async function seed(opts: SeedOptions) {
  const admin = await adminLogin()
  const suffix = uniq(opts.prefix)
  const platform = opts.platform || 'anthropic'
  const group = await createGroup(admin, {
    name: 'grp-' + suffix,
    platform,
    rate_multiplier: opts.rate ?? 1,
    ...(opts.group || {}),
  })

  const accounts: Array<Record<string, any>> = []
  const specs = opts.accounts && opts.accounts.length ? opts.accounts : [{}]
  for (let i = 0; i < specs.length; i++) {
    const a = specs[i]
    const upstreamKey = (a.credentials?.api_key as string) || 'up-' + suffix + '-' + i
    const acc = await createAccount(admin, {
      name: a.name || 'acct-' + suffix + '-' + i,
      platform: a.platform || platform,
      type: a.type || 'apikey',
      credentials: a.credentials || { api_key: upstreamKey, base_url: MOCK_UPSTREAM },
      ...(a.extra ? { extra: a.extra } : {}),
      group_ids: [group.id],
      concurrency: a.concurrency ?? 10,
      priority: a.priority ?? 1,
    })
    accounts.push({ ...acc, upstreamKey })
  }

  let channel: Record<string, any> | undefined
  if (opts.channel) {
    channel = await createChannel(admin, {
      name: opts.channel.name || 'ch-' + suffix,
      group_ids: [group.id],
      model_pricing: opts.channel.model_pricing,
    })
  }

  const email = suffix + '@v042e2e.local'
  const password = 'E2e-Passw0rd!'
  const user = await createUser(admin, {
    email,
    password,
    balance: opts.balance ?? 100,
    allowed_groups: [group.id],
  })
  const userToken = await userLogin(email, password)
  const key = await createKey(userToken, {
    name: 'key-' + suffix,
    group_id: group.id,
    custom_key: 'sk-' + suffix,
    quota: opts.quota ?? 100,
  })

  return {
    admin,
    suffix,
    platform,
    group,
    accounts,
    account: accounts[0],
    upstreamKey: accounts[0].upstreamKey as string,
    channel,
    user,
    email,
    password,
    userToken,
    key,
  }
}
