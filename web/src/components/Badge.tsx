// State and health badges: the word colored by its family.

const families: Record<string, string> = {
  applied: 'ok',
  ok: 'ok',
  done: 'ok',
  healthy: 'ok',
  merged: 'ok',
  running: 'running',
  pending: 'running',
  planned: 'running',
  stale: 'warn',
  unrefreshed: 'warn',
  unfilled: 'warn',
  compensating: 'warn',
  removing: 'warn',
  open: 'warn',
  failed: 'bad',
  invalid: 'bad',
  lost: 'bad',
  unhealthy: 'bad',
  conflict: 'bad',
}

export function Badge({ value }: { value?: string }) {
  if (!value) return <span className="muted">—</span>
  return <span className={`badge ${families[value] ?? ''}`}>{value}</span>
}
