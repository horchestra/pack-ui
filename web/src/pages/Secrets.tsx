import { useState } from 'react'
import { opPost } from '../api'
import { Load, useData } from '../components/data'
import { useT } from '../i18n'

// The secrets: names and keys, never a value (К4). A new one is written through the operator's API —
// its fields are the git credentials': username, token, ssh_key, passphrase, known_hosts.

interface SecretInfo {
  name: string
  keys: string[]
}

const fieldNames = ['username', 'token', 'ssh_key', 'passphrase', 'known_hosts'] as const

export function Secrets() {
  const t = useT()
  const [bump, setBump] = useState(0)
  const secrets = useData<SecretInfo[]>('/secrets', bump, true)
  const [name, setName] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [created, setCreated] = useState('')

  const create = () => {
    setError('')
    setCreated('')
    const body = { name, fields: Object.fromEntries(Object.entries(fields).filter(([, v]) => v)) }
    opPost<{ name: string }>('/secrets', body)
      .then(() => {
        setCreated(name)
        setName('')
        setFields({})
        setBump((b) => b + 1)
      })
      .catch((e) => setError(String(e)))
  }

  return (
    <Load state={secrets}>
      {(list) => (
        <>
          <h1>{t('secrets')}</h1>
          <div className="card">
            {list.length === 0 ? (
              <p className="muted">{t('empty')}</p>
            ) : (
              <table className="list">
                <thead>
                  <tr>
                    <th>{t('secretName')}</th>
                    <th>{t('keys')}</th>
                  </tr>
                </thead>
                <tbody>
                  {list.map((s) => (
                    <tr key={s.name}>
                      <td>
                        <code>{s.name}</code>
                      </td>
                      <td>{(s.keys ?? []).join(', ')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <div className="card">
            <h2>{t('newSecret')}</h2>
            <p>
              <input placeholder={t('secretName')} value={name} onChange={(e) => setName(e.target.value)} />
            </p>
            {fieldNames.map((f) => (
              <p key={f}>
                <input
                  placeholder={f}
                  value={fields[f] ?? ''}
                  onChange={(e) => setFields({ ...fields, [f]: e.target.value })}
                  style={{ width: '30rem' }}
                  type={f === 'token' || f === 'passphrase' ? 'password' : 'text'}
                />
              </p>
            ))}
            <p>
              <button className="primary" disabled={!name || !Object.values(fields).some(Boolean)} onClick={create}>
                {t('create')}
              </button>
              {created && <span className="muted"> {created} ✓</span>}
              {error && <span className="err"> {error}</span>}
            </p>
          </div>
        </>
      )}
    </Load>
  )
}

// The audit journal: who did what, when — the requests, the approvals, the merges, as the operator's bus
// recorded them (C8).
export function Audit() {
  const t = useT()
  const audit = useData<Record<string, unknown>[]>('/audit', undefined, true)
  return (
    <Load state={audit}>
      {(list) => (
        <>
          <h1>{t('audit')}</h1>
          {list.length === 0 ? (
            <p className="muted">{t('empty')}</p>
          ) : (
            <table className="list">
              <thead>
                <tr>
                  <th>{t('when')}</th>
                  <th>{t('kind')}</th>
                  <th>{t('user')}</th>
                  <th>{t('details')}</th>
                </tr>
              </thead>
              <tbody>
                {list.map((e, i) => {
                  const { at, kind, by, ...rest } = e
                  return (
                    <tr key={i}>
                      <td className="muted">{at ? new Date(String(at)).toLocaleString() : '—'}</td>
                      <td>{String(kind ?? '')}</td>
                      <td>{by ? String(by) : '—'}</td>
                      <td>
                        <code style={{ whiteSpace: 'pre-wrap' }}>{JSON.stringify(rest)}</code>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </>
      )}
    </Load>
  )
}
