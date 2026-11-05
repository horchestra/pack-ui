import { Link, useNavigate, useParams } from 'react-router-dom'
import { opPost, relURL, type PackRequest, type ReleaseDetail } from '../api'
import { Badge } from '../components/Badge'
import { Load, useData } from '../components/data'
import { useT } from '../i18n'

// The release card: the current and the installed SHA, what next says to do (as buttons that open a
// request), what waits for an approval, the commits of the release's directory, and the documents —
// state, history, sagas, logs, actions, facts, template, graph, values.

export function splitRel(rel: string): { project: string; path: string } {
  const i = rel.indexOf(':')
  return { project: rel.slice(0, i), path: decodeURIComponent(rel.slice(i + 1)) }
}

interface Commit {
  sha: string
  subject: string
}

export function Release() {
  const { rel = '' } = useParams()
  const relE = relURL(rel)
  const { project, path } = splitRel(rel)
  const t = useT()
  const navigate = useNavigate()
  const detail = useData<ReleaseDetail>(`/releases/${relE}`)
  const commits = useData<Commit[]>(`/repo/${project}/log?path=${encodeURIComponent(path)}`, undefined, false)
  const ask = (action: string, sha: string) =>
    opPost<PackRequest>(`/releases/${relE}/requests`, { action, sha }).then((r) => navigate(`/requests/${r.id}`))

  return (
    <Load state={detail}>
      {(d) => (
        <>
          <h1>
            {t('release')} {path}
          </h1>
          <p className="muted">
            {project} · <Badge value={d.state} /> {d.health && <Badge value={d.health} />}
          </p>
          <div className="card">
            <div className="row">
              <div>
                <div className="muted">{t('sha')}</div>
                <code>{d.sha?.slice(0, 12)}</code>
              </div>
              <div>
                <div className="muted">{t('installed')}</div>
                <code>{d.installed_sha ? d.installed_sha.slice(0, 12) : '—'}</code>
              </div>
            </div>
            <p>
              <button className="primary" onClick={() => ask('plan', d.sha)}>
                {t('plan')}
              </button>
              <button onClick={() => ask('refresh', d.sha)}>{t('refresh')}</button>
              <button onClick={() => ask('check', d.sha)}>{t('check')}</button>
              {d.installed_sha && (
                <>
                  <button onClick={() => ask('rollback', d.sha)}>{t('rollback')}</button>
                  <button className="danger" onClick={() => ask('delete', d.sha)}>
                    {t('delete')}
                  </button>
                </>
              )}
            </p>
          </div>

          {(d.waiting ?? []).length > 0 && (
            <div className="card">
              <h2>{t('waiting')}</h2>
              <table className="list">
                <tbody>
                  {d.waiting.map((w) => (
                    <tr key={w.id}>
                      <td>
                        <Link to={`/requests/${w.id}`}>{w.action}</Link>
                      </td>
                      <td>
                        <code>{w.sha.slice(0, 12)}</code>
                      </td>
                      <td>{w.by}</td>
                      <td>{new Date(w.at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="card">
            <h2>{t('commits')}</h2>
            {(commits.data ?? []).length === 0 ? (
              <p className="muted">{t('empty')}</p>
            ) : (
              <table className="list">
                <tbody>
                  {(commits.data ?? []).map((c) => (
                    <tr key={c.sha}>
                      <td>
                        <code>{c.sha.slice(0, 12)}</code>
                      </td>
                      <td>{c.subject}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <nav className="card" aria-label="documents">
            {(['status', 'history', 'sagas', 'logs', 'actions', 'facts', 'template', 'graph'] as const).map((doc) => (
              <Link key={doc} to={`/r/${rel}/doc/${doc}`} style={{ marginRight: '0.8rem' }}>
                {t(doc)}
              </Link>
            ))}
            <Link to={`/r/${rel}/values`}>{t('values')}</Link>
          </nav>
        </>
      )}
    </Load>
  )
}


