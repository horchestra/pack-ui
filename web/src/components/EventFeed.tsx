import { useEffect, useRef } from 'react'
import type { PackEvent } from '../api'

// The event feed of a job: every line of the stream — a key's status, a step's line, the saga's marks, the
// result — newest at the bottom, following while it runs.

function line(ev: PackEvent): string {
  switch (ev.type) {
    case 'plan':
      return `plan: ${ev.plan?.direction ?? ''} ${ev.plan?.to ?? ''} (${ev.plan?.steps?.length ?? 0} steps)`
    case 'node':
      return `${ev.key ?? ''} ${ev.step ?? ''} ${ev.status ?? ''}${ev.error ? ': ' + ev.error : ''}`.trim()
    case 'saga':
      return `saga ${ev.saga ?? ''} ${ev.status ?? ''}`.trim()
    case 'result':
      return ev.error ? `failed: ${ev.error}` : 'ok'
    default:
      return ev.message ?? ev.error ?? ''
  }
}

export function EventFeed({ events }: { events: PackEvent[] }) {
  const end = useRef<HTMLDivElement>(null)
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: 'end' })
  }, [events.length])
  return (
    <div className="feed card" data-testid="feed" aria-live="polite">
      {events.map((ev, i) => (
        <div key={ev.seq ?? i} className={`ev ${ev.status === 'failed' || ev.error ? 'failed' : ''}`}>
          <span className="t">{ev.time ? new Date(ev.time).toLocaleTimeString() : ''}</span>
          {line(ev)}
        </div>
      ))}
      <div ref={end} />
    </div>
  )
}
