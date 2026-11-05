import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import Form, { type IChangeEvent } from '@rjsf/core'
import validator from '@rjsf/validator-ajv8'
import type { FieldTemplateProps, WidgetProps } from '@rjsf/utils'
import { parse } from 'yaml'
import { post, opGet, relURL, ApiError } from '../api'
import { splitRel } from './Release'
import { Load, useData } from '../components/data'
import { useT, useLang } from '../i18n'

// The values form (C3): the release's JSON Schema as rjsf renders it — groups, enum, pattern, bounds,
// lengths, readOnly, defaults — plus the pack's own extensions: a compute badge, a suggestion from the
// check on the fly (render.Ask), an explain of a field, secrets only as a secretRef. The save renders
// values.yaml exactly as pack values write does (the backend's /values/write) and commits it.

interface Candidate {
  value: unknown
  source: string
  why?: string
}

interface Question {
  pointer: string
  key: string
  reason: string // required | suggested
  schema: Record<string, unknown>
  secret?: boolean
  candidates?: Candidate[]
}

interface Problem {
  kind: string
  where: string
  rule?: string
  message: string
}

interface Questions {
  questions: Question[]
  problems?: Problem[]
}

interface SecretInfo {
  name: string
  keys: string[]
}

// pointerGet/set walk a JSON Pointer ("/postgres/pg_port") in the form's document.
export function pointerGet(doc: unknown, ptr: string): unknown {
  let cur = doc as Record<string, unknown>
  for (const part of ptr.split('/').slice(1)) {
    if (cur == null || typeof cur !== 'object') return undefined
    cur = cur[part] as Record<string, unknown>
  }
  return cur
}

export function pointerSet(doc: unknown, ptr: string, value: unknown): unknown {
  const parts = ptr.split('/').slice(1)
  const clone = Array.isArray(doc) ? [...(doc as unknown[])] : { ...((doc ?? {}) as Record<string, unknown>) }
  let cur = clone as Record<string, unknown>
  for (let i = 0; i < parts.length - 1; i++) {
    const next = cur[parts[i]]
    cur[parts[i]] = Array.isArray(next) ? [...next] : { ...((next ?? {}) as Record<string, unknown>) }
    cur = cur[parts[i]] as Record<string, unknown>
  }
  cur[parts[parts.length - 1]] = value
  return clone
}

// SecretWidget is a credential field (format: password): the value is only a {secretRef: {name, key}},
// chosen from the operator's secrets or created there — a value itself never passes here.
function SecretWidget(props: WidgetProps) {
  const t = useT()
  const secrets = useData<SecretInfo[]>('/secrets', undefined, true)
  const ref = (props.value ?? {}) as { secretRef?: { name?: string; key?: string } }
  const name = ref.secretRef?.name ?? ''
  const key = ref.secretRef?.key ?? ''
  const keys = (secrets.data ?? []).find((s) => s.name === name)?.keys ?? []
  const set = (n: string, k: string) => props.onChange(n || k ? { secretRef: { name: n, key: k } } : undefined)
  return (
    <span className="secret-ref">
      <input
        list={`secrets-${props.id}`}
        value={name}
        placeholder={t('secretName')}
        onChange={(e) => set(e.target.value, key)}
        disabled={props.disabled || props.readonly}
      />
      <datalist id={`secrets-${props.id}`}>
        {(secrets.data ?? []).map((s) => (
          <option key={s.name} value={s.name} />
        ))}
      </datalist>
      <input
        list={`keys-${props.id}`}
        value={key}
        placeholder={t('secretKey')}
        onChange={(e) => set(name, e.target.value)}
        disabled={props.disabled || props.readonly}
      />
      <datalist id={`keys-${props.id}`}>
        {keys.map((k) => (
          <option key={k} value={k} />
        ))}
      </datalist>
    </span>
  )
}

// fieldTemplate adds the pack's marks to a field: a compute badge (the formula fills it unless set), an
// explain button (the pack's documentation of the field).
function makeFieldTemplate(onExplain: (ptr: string) => void, explainable: (id: string) => string | null) {
  return function FieldTemplate(props: FieldTemplateProps) {
    const t = useT()
    if (props.hidden) return <>{props.children}</>
    const schema = (props.schema ?? {}) as Record<string, unknown>
    const compute = schema['x-pack-compute']
    const ptr = explainable(props.id)
    return (
      <div className={`field${props.classNames ? ' ' + props.classNames : ''}`}>
        {props.displayLabel && props.label && (
          <label htmlFor={props.id}>
            {props.label}
            {props.required && <span className="req">*</span>}
            {compute != null && compute !== false && (
              <span className="badge" title={String(typeof compute === 'string' ? compute : 'compute')}>
                {t('computed')}
              </span>
            )}
            {ptr && (
              <button type="button" className="link" onClick={() => onExplain(ptr)} title={t('explain')}>
                ?
              </button>
            )}
          </label>
        )}
        {props.description}
        {props.children}
        {props.errors}
        {props.help}
      </div>
    )
  }
}

export function Values() {
  const { rel = '' } = useParams()
  const relE = relURL(rel)
  const { project, path } = splitRel(rel)
  const t = useT()
  const { lang } = useLang()

  const schema = useData<Record<string, unknown>>(`/releases/${relE}/schema?lang=${lang}`, undefined, true)
  const file = useData<{ content: string; sha: string }>(
    `/repo/${project}/file?path=${encodeURIComponent(path + '/values.yaml')}`,
    undefined,
    false,
  )

  const [values, setValues] = useState<unknown>({})
  const [sha, setSha] = useState('')
  const [questions, setQuestions] = useState<Questions>({ questions: [] })
  const [explain, setExplain] = useState<{ ptr: string; text: string } | null>(null)
  const [message, setMessage] = useState('')
  const [saved, setSaved] = useState('')
  const [foreign, setForeign] = useState<string[] | null>(null)
  const [error, setError] = useState('')
  const timer = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    if (!file.data) return
    setSha(file.data.sha)
    try {
      setValues(parse(file.data.content) ?? {})
    } catch {
      setValues({})
    }
  }, [file.data])

  // the check on the fly: what the values as the form holds them still ask, and what they break
  const check = useCallback(
    (v: unknown) => {
      clearTimeout(timer.current)
      timer.current = setTimeout(() => {
        post<Questions>(`/api/repo/${project}/values/check`, { path, values: v ?? {} })
          .then(setQuestions)
          .catch(() => setQuestions({ questions: [] }))
      }, 400)
    },
    [project, path],
  )

  const onChange = (e: IChangeEvent) => {
    setValues(e.formData)
    check(e.formData)
  }

  // idSchema "$"-style id → the JSON Pointer explain wants (root_openbao_ui → /openbao/ui)
  const explainable = useCallback((id: string) => {
    if (!id.startsWith('root_')) return null
    return '/' + id.slice(5).split('_').join('/')
  }, [])

  const onExplain = (ptr: string) => {
    opGet<{ text?: string; doc?: string }>(`/releases/${relE}/explain?ptr=${encodeURIComponent(ptr)}&lang=${lang}`)
      .then((d) => setExplain({ ptr, text: d.text ?? d.doc ?? JSON.stringify(d) }))
      .catch((e) => setExplain({ ptr, text: String(e) }))
  }

  const apply = (ptr: string, value: unknown) => {
    const next = pointerSet(values, ptr, value)
    setValues(next)
    check(next)
  }

  const save = (rewrite: boolean) => {
    setError('')
    setSaved('')
    post<{ sha: string }>(`/api/repo/${project}/values/write`, {
      path,
      values,
      message: message || `values of ${path}`,
      lang,
      sha,
      rewrite,
    })
      .then((r) => {
        setSha(r.sha)
        setForeign(null)
        setSaved(t('saved'))
      })
      .catch((e) => {
        if (e instanceof ApiError && e.code === 'foreign_comments') {
          setForeign([(e as Error).message])
        } else {
          setError(String(e))
        }
      })
  }

  const required = questions.questions.filter((q) => q.reason === 'required')
  const suggested = questions.questions.filter((q) => q.reason === 'suggested')

  return (
    <Load state={schema}>
      {(sc) => (
        <>
          <h1>
            {t('values')} · {path}
          </h1>
          <div className="values-layout">
            <div className="values-form">
              {file.error && <p className="muted">{t('newValuesFile')}</p>}
              <Form
                schema={sc}
                formData={values}
                validator={validator}
                onChange={onChange}
                templates={{ FieldTemplate: makeFieldTemplate(onExplain, explainable) }}
                widgets={{ PasswordWidget: SecretWidget }}
                liveValidate="onChange"
              >
                <div />
              </Form>
              <div className="save-bar">
                <input
                  value={message}
                  placeholder={t('commitMessage')}
                  onChange={(e) => setMessage(e.target.value)}
                />
                <button className="primary" onClick={() => save(false)}>
                  {t('save')}
                </button>
                {saved && <span className="ok">{saved}</span>}
                {error && <span className="err">{error}</span>}
              </div>
              {foreign && (
                <div className="card warn">
                  <p>{foreign[0]}</p>
                  <button onClick={() => save(true)}>{t('rewriteAnyway')}</button>
                </div>
              )}
            </div>
            <aside className="values-side">
              {required.length > 0 && (
                <div className="card">
                  <h2>{t('missing')}</h2>
                  <ul>
                    {required.map((q) => (
                      <li key={q.pointer}>
                        <code>{q.pointer}</code>
                        {q.secret && <span className="badge">{t('secret')}</span>}
                        {(q.candidates ?? []).map((c, i) => (
                          <button key={i} className="link" onClick={() => apply(q.pointer, c.value)}>
                            {JSON.stringify(c.value)} ({c.source})
                          </button>
                        ))}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {suggested.length > 0 && (
                <div className="card">
                  <h2>{t('suggestions')}</h2>
                  <ul>
                    {suggested.map((q) => (
                      <li key={q.pointer}>
                        <code>{q.pointer}</code>
                        {(q.candidates ?? []).map((c, i) => (
                          <button key={i} className="link" title={c.why} onClick={() => apply(q.pointer, c.value)}>
                            {JSON.stringify(c.value)}
                          </button>
                        ))}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {(questions.problems ?? []).length > 0 && (
                <div className="card warn">
                  <h2>{t('problems')}</h2>
                  <ul>
                    {(questions.problems ?? []).map((p, i) => (
                      <li key={i}>
                        <code>{p.where}</code> {p.message}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {explain && (
                <div className="card">
                  <h2>
                    {t('explain')} <code>{explain.ptr}</code>
                  </h2>
                  <pre className="explain">{explain.text}</pre>
                  <button onClick={() => setExplain(null)}>{t('close')}</button>
                </div>
              )}
            </aside>
          </div>
        </>
      )}
    </Load>
  )
}
