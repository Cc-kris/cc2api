import { execFileSync } from 'node:child_process'
import { mockReset } from '../helpers/mock'

const PSQL = ['exec', 'v042-e2e-pg', 'psql', '-U', 'postgres', '-d', 'sub2api']

function psql(args: string[]) {
  execFileSync('docker', [...PSQL, ...args], { stdio: 'inherit' })
}

/**
 * Reset the isolated E2E database before every run so the suite is repeatable.
 * The admin account, settings and migration bookkeeping are preserved; all
 * business tables and every non-admin user are removed. Redis is flushed to
 * drop auth/rate-limit caches, and the mock upstream is reset.
 */
export default async function globalSetup() {
  const block = [
    'DO $$ DECLARE r record; BEGIN ',
    "FOR r IN SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT IN ('users','settings','schema_migrations') ",
    "LOOP EXECUTE 'TRUNCATE TABLE public.' || quote_ident(r.tablename) || ' CASCADE'; END LOOP; END $$;",
  ].join('')

  psql(['-v', 'ON_ERROR_STOP=1', '-c', block])
  psql(['-v', 'ON_ERROR_STOP=1', '-c', "DELETE FROM users WHERE role <> 'admin'"])

  execFileSync('docker', ['exec', 'v042-e2e-redis', 'redis-cli', 'FLUSHDB'], { stdio: 'inherit' })
  await mockReset()
}
