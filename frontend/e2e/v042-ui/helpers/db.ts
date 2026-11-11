import { execFileSync } from 'node:child_process'

export function sql(q: string): string {
  return execFileSync(
    'docker',
    ['exec', 'v042-e2e-pg', 'psql', '-U', 'postgres', '-d', 'sub2api', '-At', '-F', '|', '-c', q],
    { encoding: 'utf8' },
  ).trim()
}

export function rows(q: string): string[][] {
  const s = sql(q)
  return s ? s.split('\n').map((l) => l.split('|')) : []
}

export function scalar(q: string): string {
  const r = rows(q)
  return r.length ? r[0][0] : ''
}
