import type { PackEvent } from '../api'

// The key graph of a running (or replayed) job: every key the stream mentioned, in the plan's order when a
// plan event named one, colored by the last status it took
// (running/done/failed/compensating/rolled-back/removing/removed).

export function keyStatuses(events: PackEvent[]): Map<string, string> {
  const out = new Map<string, string>()
  for (const ev of events) {
    if (ev.type === 'node' && ev.key && ev.status) out.set(ev.key, ev.status)
  }
  return out
}

export function keyOrder(events: PackEvent[]): string[] {
  for (const ev of events) {
    if (ev.type === 'plan' && ev.plan?.order?.length) return ev.plan.order
  }
  const seen: string[] = []
  for (const ev of events) {
    if (ev.key && !seen.includes(ev.key)) seen.push(ev.key)
  }
  return seen
}

export function Keys({ events }: { events: PackEvent[] }) {
  const statuses = keyStatuses(events)
  const order = keyOrder(events)
  const keys = order.length ? order : [...statuses.keys()]
  if (!keys.length) return null
  return (
    <div className="keys" data-testid="keys">
      {keys.map((k) => (
        <span key={k} className={`key ${statuses.get(k) ?? ''}`} title={statuses.get(k) ?? 'waiting'}>
          {k}
        </span>
      ))}
    </div>
  )
}
