import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { parse, stringify } from 'yaml'
import { put, ApiError } from '../api'
import { splitRel } from './Release'
import { useData } from '../components/data'
import { useT } from '../i18n'

// The environment form (C3): the release's env.yaml by pkg/environment's sections — the target and its
// settings, the run policy, the other targets — the YAML beside it (block), each editable, the other
// following. Saved through the same file write any file is (PUT /api/repo/{id}/file).

interface EnvDoc {
  target?: string
  lang?: string
  settings?: Record<string, string>
  run?: Record<string, unknown>
  targets?: Record<string, { target: string; settings?: Record<string, string> }>
  keys?: Record<string, unknown>
  [k: string]: unknown
}

export function Env() {
  const { rel = '' } = useParams()
  const { project, path } = splitRel(rel)
  const t = useT()
  const envPath = path + '/env.yaml'
  const file = useData<{ content: string; sha: string }>(
    `/repo/${project}/file?path=${encodeURIComponent(envPath)}`,
    undefined,
    false,
  )

  const [doc, setDoc] = useState<EnvDoc>({})
  const [yamlText, setYamlText] = useState('')
  const [yamlError, setYamlError] = useState('')
  const [sha, setSha] = useState('')
  const [message, setMessage] = useState('')
  const [status, setStatus] = useState('')

  useEffect(() => {
    if (!file.data) return
    setSha(file.data.sha)
    try {
      const d = (parse(file.data.content) ?? {}) as EnvDoc
      setDoc(d)
      setYamlText(stringify(d))
    } catch {
      setYamlText(file.data.content)
    }
  }, [file.data])

  // the form edits the document; the YAML follows
  const update = (next: EnvDoc) => {
    setDoc(next)
    setYamlText(stringify(next))
  }

  // the YAML edits back; a line that does not parse stays the user's to fix
  const onYaml = (text: string) => {
    setYamlText(text)
    try {
      setDoc((parse(text) ?? {}) as EnvDoc)
      setYamlError('')
    } catch (e) {
      setYamlError(String(e))
    }
  }

  const setSetting = (oldKey: string, key: string, value: string) => {
    const settings = { ...(doc.settings ?? {}) }
    if (oldKey !== key) delete settings[oldKey]
    if (key) settings[key] = value
    update({ ...doc, settings })
  }

  const save = () => {
    setStatus('')
    put<{ sha: string }>(`/api/repo/${project}/file`, {
      path: envPath,
      content: yamlText,
      message: message || `env of ${path}`,
      sha,
    })
      .then((r) => {
        setSha(r.sha)
        setStatus(t('saved'))
      })
      .catch((e) => setStatus(e instanceof ApiError ? `${e.code}: ${e.message}` : String(e)))
  }

  return (
    <>
      <h1>
        {t('environment')} · {path}
      </h1>
      {file.error && <p className="muted">{t('newEnvFile')}</p>}
      <div className="values-layout">
        <div className="values-form">
          <div className="card">
            <label>
              {t('target')}
              <input value={doc.target ?? ''} onChange={(e) => update({ ...doc, target: e.target.value || undefined })} />
            </label>
            <label>
              {t('language')}
              <input value={doc.lang ?? ''} onChange={(e) => update({ ...doc, lang: e.target.value || undefined })} />
            </label>
          </div>
          <div className="card">
            <h2>{t('settings')}</h2>
            {Object.entries(doc.settings ?? {}).map(([k, v]) => (
              <div className="row" key={k}>
                <input value={k} onChange={(e) => setSetting(k, e.target.value, v)} />
                <input value={v} onChange={(e) => setSetting(k, k, e.target.value)} />
                <button
                  onClick={() => {
                    const settings = { ...(doc.settings ?? {}) }
                    delete settings[k]
                    update({ ...doc, settings })
                  }}
                >
                  ×
                </button>
              </div>
            ))}
            <button onClick={() => setSetting('', `key${Object.keys(doc.settings ?? {}).length + 1}`, '')}>
              + {t('addSetting')}
            </button>
          </div>
          <div className="card">
            <h2>{t('targets')}</h2>
            {Object.entries(doc.targets ?? {}).map(([name, tg]) => (
              <div className="row" key={name}>
                <code>{name}</code>
                <input
                  value={tg.target}
                  onChange={(e) => update({ ...doc, targets: { ...(doc.targets ?? {}), [name]: { ...tg, target: e.target.value } } })}
                />
                <button
                  onClick={() => {
                    const targets = { ...(doc.targets ?? {}) }
                    delete targets[name]
                    update({ ...doc, targets })
                  }}
                >
                  ×
                </button>
              </div>
            ))}
            <button
              onClick={() => update({ ...doc, targets: { ...(doc.targets ?? {}), [`target${Object.keys(doc.targets ?? {}).length + 1}`]: { target: '' } } })}
            >
              + {t('addTarget')}
            </button>
          </div>
        </div>
        <aside className="values-side">
          <div className="card">
            <h2>YAML</h2>
            <textarea className="yaml" rows={24} value={yamlText} onChange={(e) => onYaml(e.target.value)} />
            {yamlError && <p className="err">{yamlError}</p>}
          </div>
        </aside>
      </div>
      <div className="save-bar">
        <input value={message} placeholder={t('commitMessage')} onChange={(e) => setMessage(e.target.value)} />
        <button className="primary" onClick={save} disabled={!!yamlError}>
          {t('save')}
        </button>
        {status && <span className="ok">{status}</span>}
      </div>
    </>
  )
}
