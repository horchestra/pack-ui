import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { jobEvents, opPost, type Job, type PackEvent } from '../api'
import { Badge } from '../components/Badge'
import { EventFeed } from '../components/EventFeed'
import { Keys } from '../components/Keys'
import { Json } from '../components/Json'
import { Load, useData } from '../components/data'
import { useT } from '../i18n'

// The job live: the events stream in by SSE (a reconnect resumes from the last seq — the proxy and the
// operator keep Last-Event-ID), the keys take their colors, the feed follows, the result ends it; a lost
// job offers apply/rollback (its argv was never answered).

export function JobPage() {
  const { id = '' } = useParams()
  const t = useT()
  const [reload, setReload] = useState(0)
  const job = useData<Job>(`/jobs/${id}`, reload)
  const [events, setEvents] = useState<PackEvent[]>([])
  const [result, setResult] = useState<PackEvent | null>(null)

  useEffect(() => {
    setEvents([])
    setResult(null)
    const close = jobEvents(
      id,
      (ev) => {
        if (ev.type === 'result') setResult(ev)
        setEvents((all) => [...all, ev])
      },
      undefined,
    )
    return close
  }, [id, reload])

  const cancel = () => opPost(`/jobs/${id}/cancel`).then(() => setReload((n) => n + 1))

  const running = job.data && !['done', 'failed', 'lost'].includes(job.data.state)
  return (
    <Load state={job}>
      {(j) => (
        <>
          <h1>
            {t('job')} {j.id.slice(0, 12)}
          </h1>
          <p className="muted">
            <code>{(j.argv ?? []).join(' ')}</code> · <Badge value={j.state} />
          </p>
          <Keys events={events} />
          <h2>{t('events')}</h2>
          <EventFeed events={events} />
          {running && (
            <p>
              <button className="danger" onClick={cancel}>
                {t('interrupt')}
              </button>
            </p>
          )}
          {j.state === 'lost' && (
            <p className="failure" role="alert">
              {t('lost')} — {t('apply')} / {t('rollback')}: {t('release')}
            </p>
          )}
          {result && (
            <div className="card">
              <h2>{t('result')}</h2>
              <Json doc={result.result ?? result.error} />
            </div>
          )}
        </>
      )}
    </Load>
  )
}
