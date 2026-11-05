import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { opPost } from '../api'
import { Badge } from '../components/Badge'
import { Load, useData } from '../components/data'
import { useT } from '../i18n'

// The change requests of a project (C4): the list, the newest first, and the form to open one — a branch,
// a title; a branch the remote has not is made at the main branch's head.

export interface ChangeBrief {
  id: string
  project: string
  branch: string
  author: string
  title: string
  state: string // open | merged | closed | conflict
  sha: string
  at: string
}

export function Changes() {
  const { project = '' } = useParams()
  const t = useT()
  const navigate = useNavigate()
  const [reload, setReload] = useState(0)
  const list = useData<ChangeBrief[]>(`/changes?project=${encodeURIComponent(project)}`, reload)
  const [branch, setBranch] = useState('')
  const [title, setTitle] = useState('')
  const [error, setError] = useState('')

  const open = () => {
    setError('')
    opPost<ChangeBrief>('/changes', { project, branch, title })
      .then((c) => navigate(`/changes/${c.id}`))
      .catch((e) => setError(String(e)))
  }

  return (
    <>
      <h1>
        {t('changes')} · {project}
      </h1>
      <div className="card">
        <h2>{t('openChange')}</h2>
        <div className="row">
          <input value={branch} placeholder={t('branch')} onChange={(e) => setBranch(e.target.value)} />
          <input value={title} placeholder={t('title')} onChange={(e) => setTitle(e.target.value)} />
          <button className="primary" disabled={!branch || !title} onClick={open}>
            {t('create')}
          </button>
        </div>
        {error && <p className="err">{error}</p>}
      </div>
      <Load state={list}>
        {(changes) =>
          changes.length === 0 ? (
            <p className="muted">{t('empty')}</p>
          ) : (
            <table className="list">
              <thead>
                <tr>
                  <th>{t('title')}</th>
                  <th>{t('branch')}</th>
                  <th>{t('state')}</th>
                  <th>{t('user')}</th>
                </tr>
              </thead>
              <tbody>
                {changes.map((c) => (
                  <tr key={c.id}>
                    <td>
                      <Link to={`/changes/${c.id}`}>{c.title}</Link>
                    </td>
                    <td>
                      <code>{c.branch}</code>
                    </td>
                    <td>
                      <Badge value={c.state} />
                    </td>
                    <td>{c.author}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        }
      </Load>
      <p>
        <button onClick={() => setReload(reload + 1)}>{t('refresh')}</button>
      </p>
    </>
  )
}
