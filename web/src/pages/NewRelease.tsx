import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { post, type PackRequest, type Project } from '../api'
import { useData } from '../components/data'
import { useT, useLang } from '../i18n'

// A new release (C5): the pack — an oci:// reference or a directory of the repository (an authored
// project) — where the release lives, its environment (the target, where its program or image comes from;
// the runtime of the job is the project's own setting). pack-ui runs pack init in a checkout, commits the
// files as the user and asks the operator the refresh; the answer is the request's page.

export function NewRelease() {
  const { project = '' } = useParams()
  const t = useT()
  const { lang } = useLang()
  const navigate = useNavigate()
  const projects = useData<Project[]>('/projects', undefined, false)
  const flow = (projects.data ?? []).find((p) => p.id === project)?.flow ?? 'direct'

  const [form, setForm] = useState({
    pack: '',
    path: '',
    target: '',
    target_source: '',
    target_image: '',
    runtime: '',
    message: '',
    branch: '',
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })

  const known = ['docker', 'ansible', 'k8s', 'memory']

  const submit = () => {
    setBusy(true)
    setError('')
    post<{ request: PackRequest }>(`/api/repo/${project}/init`, {
      pack: form.pack,
      path: form.path,
      target: form.target,
      target_source: form.target_source || undefined,
      target_image: form.target_image || undefined,
      runtime: form.runtime || undefined,
      lang,
      message: form.message || undefined,
      branch: form.branch || undefined,
    })
      .then((r) => navigate(`/requests/${r.request.id}`))
      .catch((e) => {
        setBusy(false)
        setError(String(e))
      })
  }

  return (
    <>
      <h1>
        {t('newRelease')} · {project}
      </h1>
      <div className="card">
        <label>
          {t('pack')}
          <input
            value={form.pack}
            placeholder="oci://registry/packs/openbao:2.1.0 or packs/kv"
            onChange={set('pack')}
          />
        </label>
        <label>
          {t('path')}
          <input value={form.path} placeholder="releases/prod/db" onChange={set('path')} />
        </label>
        <label>
          {t('target')}
          <input list="known-targets" value={form.target} onChange={set('target')} />
          <datalist id="known-targets">
            {known.map((k) => (
              <option key={k} value={k} />
            ))}
          </datalist>
        </label>
        <label>
          {t('targetSource')}
          <input value={form.target_source} placeholder="https://… or oci://…" onChange={set('target_source')} />
        </label>
        <label>
          {t('targetImage')}
          <input value={form.target_image} placeholder="oci://…" onChange={set('target_image')} />
        </label>
        <label>
          {t('runtime')}
          <select value={form.runtime} onChange={set('runtime')}>
            <option value="">native</option>
            <option value="docker">docker</option>
            <option value="k8s">k8s</option>
          </select>
        </label>
        {flow === 'review' && (
          <label>
            {t('branch')}
            <input value={form.branch} onChange={set('branch')} />
          </label>
        )}
        <label>
          {t('commitMessage')}
          <input value={form.message} onChange={set('message')} />
        </label>
        <p>
          <button className="primary" disabled={busy || !form.pack || !form.path || !form.target} onClick={submit}>
            {t('create')}
          </button>
          {busy && <span className="muted"> {t('loading')}</span>}
        </p>
        {error && <div className="failure">{error}</div>}
      </div>
    </>
  )
}
