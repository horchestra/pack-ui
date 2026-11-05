import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { parse } from 'yaml'
import { get, opPost, post, relURL, type PackRequest } from '../api'
import { useT, useLang } from '../i18n'
import { pointerSet } from './Values'

// The release's failure as pkg/diagnosis says it, and what to do about it: a patch is committed to the
// values only after the user confirms it; a command opens a deploy request of the operator's. Nothing
// runs on its own (C7 — the model's and the page's fixes are the user's to confirm).

interface Remedy {
  id: string
  patch?: string
  value?: unknown
  command?: string
  why?: string
}

interface Case {
  saga?: string
  key?: string
  error: string
  class: string
  diagnosis?: string
  cause: string
  evidence?: string[]
  truncated?: boolean
  remedies?: Remedy[]
}

// A remedy's command to the deploy request that runs it; the rest are pages the page points at.
const commandAction: Record<string, string> = {
  'pack refresh': 'refresh',
  'pack discover': 'discover',
}

export function Diagnose({ project, path, sha }: { project: string; path: string; sha: string }) {
  const t = useT()
  const { lang } = useLang()
  const navigate = useNavigate()
  const relE = relURL(`${project}:${path}`)
  const [kc, setKc] = useState<Case | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [done, setDone] = useState('')

  const run = () => {
    setError('')
    setKc(null)
    setDone('')
    post<Case>(`/api/repo/${project}/diagnose`, { path, lang }).then(setKc).catch((e) => setError(String(e)))
  }

  const applyPatch = (r: Remedy) => {
    if (!r.patch) return
    if (!window.confirm(`${r.patch} = ${JSON.stringify(r.value)}\n\n${r.why ?? ''}`)) return
    setBusy(r.id)
    setError('')
    get<{ content: string; sha: string }>(`/api/repo/${project}/file?path=${encodeURIComponent(path + '/values.yaml')}`)
      .then((f) => {
        const values = f.content ? (parse(f.content) ?? {}) : {}
        return post<{ sha: string }>(`/api/repo/${project}/values/write`, {
          path,
          values: pointerSet(values, r.patch!, r.value),
          message: `diagnose ${kc?.diagnosis ?? kc?.class}: ${r.patch}`,
          lang,
          sha: f.sha,
        })
      })
      .then(() => setDone(r.id))
      .catch((e) => setError(String(e)))
      .finally(() => setBusy(''))
  }

  const runRemedy = (r: Remedy) => {
    if (!r.command) return
    if (r.command === 'pack fill') {
      navigate(`/r/${encodeURIComponent(project + ':' + path)}/values`)
      return
    }
    const action = commandAction[r.command]
    if (!action) {
      navigate(`/r/${encodeURIComponent(project + ':' + path)}/doc/logs`)
      return
    }
    setBusy(r.id)
    setError('')
    opPost<PackRequest>(`/releases/${relE}/requests`, { action, sha })
      .then((req) => navigate(`/requests/${req.id}`))
      .catch((e) => {
        setError(String(e))
        setBusy('')
      })
  }

  return (
    <div className="card">
      <h2>{t('diagnose')}</h2>
      {!kc && (
        <p>
          <button onClick={run}>{t('diagnose')}</button>
        </p>
      )}
      {error && <p className="err">{error}</p>}
      {kc && (
        <>
          <p>
            <strong>{kc.diagnosis ?? kc.class}</strong> — {kc.cause}
            {kc.key && <code style={{ marginLeft: '0.5rem' }}>{kc.key}</code>}
          </p>
          <p className="muted">{kc.error}</p>
          {(kc.evidence ?? []).length > 0 && (
            <details>
              <summary>{t('evidence')}</summary>
              <pre>{kc.evidence!.join('\n')}</pre>
            </details>
          )}
          {(kc.remedies ?? []).length > 0 && (
            <table className="list">
              <tbody>
                {kc.remedies!.map((r) => (
                  <tr key={r.id}>
                    <td>
                      {r.patch ? (
                        <code>
                          {r.patch} = {JSON.stringify(r.value)}
                        </code>
                      ) : (
                        <code>{r.command || r.why}</code>
                      )}
                      {r.why && <div className="muted">{r.why}</div>}
                    </td>
                    <td>
                      {done === r.id ? (
                        t('fixApplied')
                      ) : r.patch ? (
                        <button disabled={busy === r.id} onClick={() => applyPatch(r)}>
                          {t('applyFix')}
                        </button>
                      ) : r.command ? (
                        <button disabled={busy === r.id} onClick={() => runRemedy(r)}>
                          {t('runCommand')}
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  )
}
