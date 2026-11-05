import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { opPost, relURL, type GraphResult, type PackEvent, type PackRequest } from '../api'
import { Json } from '../components/Json'
import { Load, useData } from '../components/data'
import { Mermaid, mermaidOf } from '../components/Mermaid'
import { Badge } from '../components/Badge'
import { EventFeed } from '../components/EventFeed'
import { useT, useLang, type Key } from '../i18n'

// The documents of a release the release card links to: state (status), history, the sagas' journal, the
// logs, the pack's actions (each run through a request — a plan and an approval, as every change), the
// facts (the target), the template, the graph.

const docs: Key[] = ['status', 'history', 'sagas', 'logs', 'actions', 'facts', 'template', 'graph']

export function Docs() {
  const { rel = '', doc = 'status' } = useParams()
  const relE = relURL(rel)
  const t = useT()
  const { lang } = useLang()
  const navigate = useNavigate()
  const [runMsg, setRunMsg] = useState('')
  const suffix = doc === 'schema' || doc === 'explain' ? `?lang=${lang}` : ''
  const state = useData<unknown>(doc === 'graph' || doc === 'sagas' ? null : `/releases/${relE}/${doc}${suffix}`)
  const graph = useData<GraphResult>(doc === 'graph' ? `/releases/${relE}/graph` : null)

  const run = (name: string) =>
    opPost<PackRequest>(`/releases/${relE}/requests`, { action: 'run', params: { run: name } })
      .then((r) => navigate(`/requests/${r.id}`))
      .catch((e) => setRunMsg(e.message))

  return (
    <>
      <h1>{t(doc as Key)}</h1>
      <nav className="card" aria-label="documents">
        {docs.map((d) => (
          <Link key={d} to={`/r/${rel}/doc/${d}`} style={{ marginRight: '0.8rem' }}>
            {t(d)}
          </Link>
        ))}
      </nav>
      {doc === 'graph' ? (
        <Load state={graph}>{(g) => <Mermaid text={mermaidOf(g)} />}</Load>
      ) : doc === 'sagas' ? (
        <Sagas rel={rel} />
      ) : doc === 'actions' ? (
        <Load state={state}>
          {(d) => (
            <>
              {runMsg && <p role="alert">{runMsg}</p>}
              <Actions doc={d} onRun={run} />
            </>
          )}
        </Load>
      ) : (
        <Load state={state}>{(d) => <Json doc={d} />}</Load>
      )}
    </>
  )
}

// The actions of the pack: buttons that open a run request; the document itself below.
function Actions({ doc, onRun }: { doc: unknown; onRun: (name: string) => void }) {
  const actions = (doc as { actions?: { name?: string; text?: string }[] })?.actions ?? []
  return (
    <>
      {actions.length > 0 && (
        <div className="card">
          {actions.map((a, i) => (
            <p key={i}>
              <button onClick={() => a.name && onRun(a.name)}>{a.name ?? 'run'}</button>{' '}
              <span className="muted">{a.text ?? ''}</span>
            </p>
          ))}
        </div>
      )}
      <Json doc={doc} />
    </>
  )
}

// The sagas: the list, and a chosen saga's events replayed (the SSE of a past saga closes at its end).
function Sagas({ rel }: { rel: string }) {
  const t = useT()
  const state = useData<{ id: string; at: string; outcome: string }[]>(`/releases/${relURL(rel)}/sagas`)
  const [open, setOpen] = useState<string | null>(null)
  return (
    <Load state={state}>
      {(sagas) => (
        <>
          <table className="list">
            <thead>
              <tr>
                <th>saga</th>
                <th>{t('state')}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {sagas.map((s) => (
                <tr key={s.id}>
                  <td>
                    <button onClick={() => setOpen(open === s.id ? null : s.id)}>
                      <code>{s.id}</code>
                    </button>
                  </td>
                  <td>
                    <Badge value={s.outcome} />
                  </td>
                  <td>{s.at ? new Date(s.at).toLocaleString() : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {open && <SagaEvents rel={rel} saga={open} />}
        </>
      )}
    </Load>
  )
}

function SagaEvents({ rel, saga }: { rel: string; saga: string }) {
  const [events, setEvents] = useState<PackEvent[]>([])
  useEffect(() => {
    setEvents([])
    const es = new EventSource(`/api/op/releases/${relURL(rel)}/sagas/${encodeURIComponent(saga)}/events`)
    es.onmessage = (m) => setEvents((all) => [...all, JSON.parse(m.data) as PackEvent])
    es.onerror = () => es.close() // the replay's end closes the stream
    return () => es.close()
  }, [rel, saga])
  return <EventFeed events={events} />
}
