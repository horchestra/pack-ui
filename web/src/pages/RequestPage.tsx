import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  opPost,
  relID,
  type GraphResult,
  type PackEvent,
  type Plan,
  type ReleaseDetail,
  type RequestDetail,
  type TemplateFile,
  type TemplateResult,
} from '../api'
import { Badge } from '../components/Badge'
import { Diff } from '../components/Diff'
import { Load, useData } from '../components/data'
import { Mermaid, mermaidOf } from '../components/Mermaid'
import { useT } from '../i18n'

// The request page: the plan that waits for an approval — its changes, steps, risks, the digest and the
// SHA; the rendered files with a diff against the installed ones; the graph. The approval approves exactly
// the digest shown on exactly the SHA shown (К4: the operator refuses anything else).

function PlanView({ plan }: { plan: Plan }) {
  const t = useT()
  return (
    <div data-testid="plan">
      <p>
        <strong>{plan.direction}</strong> {plan.pack} {plan.from && `${plan.from} → `}
        {plan.to}
      </p>
      {(plan.changes ?? []).length > 0 && (
        <>
          <h3>{t('changes2')}</h3>
          <table className="list">
            <thead>
              <tr>
                <th>{t('release')}</th>
                <th>component</th>
                <th>kind</th>
                <th>from → to</th>
              </tr>
            </thead>
            <tbody>
              {plan.changes.map((c, i) => (
                <tr key={i}>
                  <td>
                    <code>{c.key}</code>
                  </td>
                  <td>{c.component}</td>
                  <td>{c.kind}</td>
                  <td>
                    {c.from ?? ''} → {c.to ?? ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
      <h3>{t('steps')}</h3>
      <table className="list">
        <tbody>
          {(plan.steps ?? []).map((s) => (
            <tr key={s.n}>
              <td>{s.n}</td>
              <td>
                <code>{s.key}</code>
              </td>
              <td>{s.kind}</td>
              <td>{s.detail ?? s.name ?? ''}</td>
              <td>{s.irreversible ? <Badge value="irreversible" /> : null}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {(plan.risks ?? []).length > 0 && (
        <>
          <h3>{t('risks')}</h3>
          <ul>
            {(plan.risks ?? []).map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}

// TemplateDiff renders the files of the request's sha with a diff against the installed ones (the
// installed sha's template; a first install diffs against nothing).
function TemplateDiff({ rel, sha }: { rel: string; sha: string }) {
  const t = useT()
  const fresh = useData<TemplateResult>(`/releases/${rel}/template?sha=${sha}`)
  const detail = useData<ReleaseDetail>(`/releases/${rel}`)
  const installedSHA = detail.data?.installed_sha
  const installed = useData<TemplateResult>(installedSHA ? `/releases/${rel}/template?sha=${installedSHA}` : null)
  return (
    <Load state={fresh}>
      {(doc) => {
        const base = new Map<string, string>()
        for (const f of installed.data?.files ?? []) {
          if (f.content) base.set(f.path + (f.key ?? ''), f.content)
        }
        return (
          <>
            <h3>{t('template')}</h3>
            {(doc.files ?? []).map((f: TemplateFile, i: number) => (
              <details key={i}>
                <summary>
                  <code>{f.path}</code> {f.secret && <span className="muted">(secret)</span>}
                </summary>
                {f.content ? (
                  <Diff from={base.get(f.path + (f.key ?? '')) ?? ''} to={f.content} />
                ) : (
                  <p className="muted">{f.secret}</p>
                )}
              </details>
            ))}
          </>
        )
      }}
    </Load>
  )
}

function Graph({ rel, sha }: { rel: string; sha: string }) {
  const t = useT()
  const state = useData<GraphResult>(`/releases/${rel}/graph?sha=${sha}`)
  return (
    <Load state={state}>
      {(g) => (
        <>
          <h3>{t('graph')}</h3>
          <Mermaid text={mermaidOf(g)} />
        </>
      )}
    </Load>
  )
}

// planOfJob replays a job's events to its plan — the plan the approval would be for.
async function planOfJob(jobID: string): Promise<Plan | null> {
  // the saga's past events come from the events endpoint as a replay (the stream closes at the present)
  return new Promise((resolve) => {
    const es = new EventSource(`/api/op/jobs/${encodeURIComponent(jobID)}/events`)
    let found: Plan | null = null
    es.onmessage = (m) => {
      const ev = JSON.parse(m.data) as PackEvent
      if (ev.type === 'plan' && ev.plan) found = ev.plan
      if (ev.type === 'result') {
        es.close()
        resolve(found)
      }
    }
    es.onerror = () => {
      es.close()
      resolve(found)
    }
  })
}

export function RequestPage() {
  const { id = '' } = useParams()
  const t = useT()
  const [reload, setReload] = useState(0)
  const state = useData<RequestDetail>(`/requests/${id}`, reload)
  const [plan, setPlan] = useState<Plan | null>(null)
  const [msg, setMsg] = useState('')

  const approve = (d: RequestDetail) =>
    opPost(`/requests/${id}/approve`, { digest: d.digest, sha: d.sha })
      .then(() => {
        setMsg(t('approved'))
        setReload((n) => n + 1)
      })
      .catch((e) => setMsg(e.message))

  return (
    <Load state={state}>
      {(d) => {
        const rel = relID(d.repo, d.target.release ?? '')
        if (!plan && d.jobs?.length) {
          void planOfJob(d.jobs[0].id).then(setPlan)
        }
        return (
          <>
            <h1>
              {t('request')} {d.action}
            </h1>
            <p className="muted">
              <Link to={`/r/${rel}`}>{d.target.release}</Link> · <code>{d.sha.slice(0, 12)}</code> · {d.by} ·{' '}
              <Badge value={d.state} />
            </p>
            {d.digest && (
              <div className="card">
                <p>
                  {t('digest')}: <code data-testid="digest">{d.digest}</code>
                </p>
                <p>
                  {t('sha')}: <code>{d.sha}</code>
                </p>
                <button className="primary" onClick={() => approve(d)}>
                  {t('approve')}
                </button>
                {msg && <p role="status">{msg}</p>}
              </div>
            )}
            <div className="card">{plan ? <PlanView plan={plan} /> : <p className="muted">{t('loading')}</p>}</div>
            <div className="card">
              <TemplateDiff rel={rel} sha={d.sha} />
            </div>
            <div className="card">
              <Graph rel={rel} sha={d.sha} />
            </div>
            <div className="card">
              <h3>{t('jobs')}</h3>
              <table className="list">
                <tbody>
                  {(d.jobs ?? []).map((j) => (
                    <tr key={j.id}>
                      <td>
                        <Link to={`/jobs/${j.id}`}>{j.id.slice(0, 12)}</Link>
                      </td>
                      <td>
                        <code>{(j.argv ?? []).join(' ')}</code>
                      </td>
                      <td>
                        <Badge value={j.state} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )
      }}
    </Load>
  )
}
