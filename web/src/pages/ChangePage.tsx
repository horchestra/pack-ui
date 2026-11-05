import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { get, opPost, relID, ApiError, type Plan, type PackRequest } from '../api'
import { Badge } from '../components/Badge'
import { Diff } from '../components/Diff'
import { Json } from '../components/Json'
import { Load, useData } from '../components/data'
import { useT } from '../i18n'
import type { ChangeBrief } from './Changes'

// A change request (C4): the diff of its branch against main, the plans of the releases it touches (the
// operator planned them when the branch moved), the checks one asks of them, the comments, and the merge
// and close the rights allow — every one through the operator's /api/changes.

interface ChangePlan {
  release: string
  sha: string
  digest: string
  plan: Plan
}

interface Comment {
  author: string
  body: string
  at: string
}

interface ChangeDetail extends ChangeBrief {
  plans: ChangePlan[]
  comments: Comment[]
}

interface FileDiff {
  path: string
  from: string
  to: string
}

export function ChangePage() {
  const { id = '' } = useParams()
  const t = useT()
  const navigate = useNavigate()
  const [reload, setReload] = useState(0)
  const detail = useData<ChangeDetail>(`/changes/${id}`, reload)
  const [diff, setDiff] = useState<FileDiff[] | null>(null)
  const [diffFor, setDiffFor] = useState('')
  const [comment, setComment] = useState('')
  const [error, setError] = useState('')

  // the diff, once the change says its project and branch
  if (detail.data && diffFor !== detail.data.id) {
    setDiffFor(detail.data.id)
    get<FileDiff[]>(`/api/repo/${detail.data.project}/diff?head=${encodeURIComponent(detail.data.branch)}`)
      .then(setDiff)
      .catch(() => setDiff([]))
  }

  const act = (what: 'merge' | 'close') => {
    setError('')
    opPost(`/changes/${id}/${what}`)
      .then(() => setReload(reload + 1))
      .catch((e) => setError(e instanceof ApiError ? `${e.code}: ${e.message}` : String(e)))
  }

  const send = () => {
    setError('')
    opPost(`/changes/${id}/comments`, { body: comment })
      .then(() => {
        setComment('')
        setReload(reload + 1)
      })
      .catch((e) => setError(String(e)))
  }

  const check = (relE: string, sha: string) => {
    opPost<PackRequest>(`/releases/${relE}/requests`, { action: 'check', sha })
      .then((r) => navigate(`/requests/${r.id}`))
      .catch((e) => setError(String(e)))
  }

  return (
    <Load state={detail}>
      {(c) => (
        <>
          <h1>
            {c.title} <Badge value={c.state} />
          </h1>
          <p className="muted">
            {c.project} · <code>{c.branch}</code> · {c.author} · {new Date(c.at).toLocaleString()}
          </p>
          {c.state === 'open' && (
            <p>
              <button className="primary" onClick={() => act('merge')}>
                {t('merge')}
              </button>
              <button onClick={() => act('close')}>{t('close')}</button>
            </p>
          )}
          {error && <div className="failure">{error}</div>}

          <div className="card">
            <h2>{t('changes2')}</h2>
            {diff == null ? (
              <p className="muted">{t('loading')}</p>
            ) : diff.length === 0 ? (
              <p className="muted">{t('empty')}</p>
            ) : (
              diff.map((f) => (
                <details key={f.path}>
                  <summary>
                    <code>{f.path}</code>
                  </summary>
                  <Diff from={f.from} to={f.to} />
                </details>
              ))
            )}
          </div>

          {c.plans.length > 0 && (
            <div className="card">
              <h2>{t('plan')}</h2>
              {c.plans.map((p) => (
                <details key={p.release}>
                  <summary>
                    <Link to={`/r/${relID(c.project, p.release)}`}>{p.release}</Link> · {p.plan.direction}{' '}
                    {p.plan.from ? `${p.plan.from} → ` : ''}
                    {p.plan.to} · {t('digest')} <code>{p.digest.slice(0, 12)}</code>
                    {(p.plan.risks ?? []).length > 0 && (
                      <>
                        {' '}
                        · <Badge value="warn" /> {(p.plan.risks ?? []).length} {t('risks').toLowerCase()}
                      </>
                    )}
                  </summary>
                  {(p.plan.risks ?? []).length > 0 && (
                    <ul>
                      {(p.plan.risks ?? []).map((r, i) => (
                        <li key={i}>{r}</li>
                      ))}
                    </ul>
                  )}
                  <Json doc={p.plan} />
                  {c.state === 'open' && (
                    <button onClick={() => check(relID(c.project, p.release), p.sha)}>{t('check')}</button>
                  )}
                </details>
              ))}
            </div>
          )}

          <div className="card">
            <h2>{t('comment')}</h2>
            {c.comments.length === 0 ? (
              <p className="muted">{t('empty')}</p>
            ) : (
              <ul className="comments">
                {c.comments.map((cm, i) => (
                  <li key={i}>
                    <span className="muted">
                      {cm.author} · {new Date(cm.at).toLocaleString()}
                    </span>
                    <div>{cm.body}</div>
                  </li>
                ))}
              </ul>
            )}
            {c.state === 'open' && (
              <div className="row">
                <input value={comment} onChange={(e) => setComment(e.target.value)} />
                <button disabled={!comment} onClick={send}>
                  {t('send')}
                </button>
              </div>
            )}
          </div>
        </>
      )}
    </Load>
  )
}
