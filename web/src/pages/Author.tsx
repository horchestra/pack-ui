import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { get, post, put, ApiError } from '../api'
import { useT } from '../i18n'

// The author's page (C6): a new pack from a spec, the pack's files edited, author check's verdict
// (generation, problems with the source's file:line, scenarios), the docs skeleton, the CEL sandbox, the
// ref and the examples, and the way to a release of the pack. Nothing here is a deploy — it runs in
// pack-ui, not through the operator.

interface Problem {
  pack: string
  path: string
  message: string
  warning?: boolean
  source?: string
}

interface ScenarioResult {
  name: string
  ok: boolean
  failures?: string[]
  skipped?: boolean
}

interface CheckResult {
  ok: boolean
  lang: string
  generate?: string
  problems?: Problem[]
  scenarios?: ScenarioResult[]
}

export function Author() {
  const { project = '' } = useParams()
  const t = useT()

  // new pack
  const [spec, setSpec] = useState('')
  const [lang, setLang] = useState('go')
  const [dir, setDir] = useState('')
  const [news, setNews] = useState('')

  // the pack's work
  const [packPath, setPackPath] = useState('')
  const [check, setCheck] = useState<CheckResult | null>(null)
  const [docsLang, setDocsLang] = useState('ru')
  const [docsText, setDocsText] = useState('')

  // the file editor
  const [filePath, setFilePath] = useState('')
  const [fileContent, setFileContent] = useState('')
  const [fileSHA, setFileSHA] = useState('')
  const [fileStatus, setFileStatus] = useState('')

  // the sandbox
  const [expr, setExpr] = useState('')
  const [celValues, setCelValues] = useState('')
  const [celFacts, setCelFacts] = useState('')
  const [celOut, setCelOut] = useState('')

  // ref and examples
  const [topic, setTopic] = useState('struct')
  const [refText, setRefText] = useState('')
  const [example, setExample] = useState('postgres')
  const [exampleFiles, setExampleFiles] = useState<Record<string, string>>({})

  const [error, setError] = useState('')
  const fail = (e: unknown) => setError(e instanceof ApiError ? `${e.code}: ${e.message}` : String(e))

  const scaffold = () => {
    setError('')
    let parsed: unknown
    try {
      parsed = JSON.parse(spec)
    } catch {
      setError('the spec is JSON here (yaml.parse of it is the values page’s way; paste JSON)')
      return
    }
    post(`/api/repo/${project}/author/scaffold`, { spec: parsed, lang, dir, message: `scaffold ${dir}` })
      .then(() => setNews(`${dir} ✓`))
      .catch(fail)
  }

  const runCheck = () => {
    setError('')
    setCheck(null)
    post<CheckResult>(`/api/repo/${project}/author/check`, { path: packPath }).then(setCheck).catch(fail)
  }

  const skeleton = () => {
    setError('')
    post<{ text: string }>(`/api/repo/${project}/author/docs`, { path: packPath, lang: docsLang })
      .then((r) => setDocsText(r.text))
      .catch(fail)
  }

  const loadFile = () => {
    setError('')
    get<{ content: string; sha: string }>(`/api/repo/${project}/file?path=${encodeURIComponent(filePath)}`)
      .then((f) => {
        setFileContent(f.content)
        setFileSHA(f.sha)
        setFileStatus('')
      })
      .catch(fail)
  }

  const saveFile = () => {
    setError('')
    put<{ sha: string }>(`/api/repo/${project}/file`, {
      path: filePath,
      content: fileContent,
      sha: fileSHA,
      message: `edit ${filePath}`,
    })
      .then((r) => {
        setFileSHA(r.sha)
        setFileStatus(t('saved'))
      })
      .catch(fail)
  }

  const evalCEL = () => {
    setError('')
    post<{ value: unknown }>(`/api/repo/${project}/author/cel`, {
      path: packPath,
      expr,
      values: celValues ? JSON.parse(celValues) : undefined,
      facts: celFacts ? JSON.parse(celFacts) : undefined,
    })
      .then((r) => setCelOut(JSON.stringify(r.value, null, 2)))
      .catch(fail)
  }

  const showRef = () => {
    setError('')
    get<{ text: string }>(`/api/author/ref?topic=${topic}&lang=${lang}`).then((r) => setRefText(r.text)).catch(fail)
  }

  const showExample = () => {
    setError('')
    get<{ files: Record<string, string> }>(`/api/author/example?name=${example}&lang=${lang}`)
      .then((r) => setExampleFiles(r.files))
      .catch(fail)
  }

  return (
    <>
      <h1>
        {t('author')} · {project}
      </h1>
      {error && <div className="failure">{error}</div>}

      <div className="card">
        <h2>{t('newPack')}</h2>
        <div className="row">
          <select value={lang} onChange={(e) => setLang(e.target.value)}>
            <option>go</option>
            <option>python</option>
            <option>java</option>
          </select>
          <input value={dir} placeholder="packs/my-pack" onChange={(e) => setDir(e.target.value)} />
          <button disabled={!spec || !dir} onClick={scaffold}>
            {t('create')}
          </button>
          {news && <span className="ok">{news}</span>}
        </div>
        <textarea
          className="yaml"
          rows={8}
          placeholder='{"name":"my-pack","version":"0.1.0","fields":[{"name":"port","type":"integer","required":true}]}'
          value={spec}
          onChange={(e) => setSpec(e.target.value)}
        />
      </div>

      <div className="card">
        <h2>{t('pack')}</h2>
        <div className="row">
          <input value={packPath} placeholder="packs/kv" onChange={(e) => setPackPath(e.target.value)} />
          <button disabled={!packPath} onClick={runCheck}>
            {t('authorCheck')}
          </button>
          <select value={docsLang} onChange={(e) => setDocsLang(e.target.value)}>
            <option>ru</option>
            <option>en</option>
          </select>
          <button disabled={!packPath} onClick={skeleton}>
            docs skeleton
          </button>
          <Link to={`/p/${project}/new-release`}>{t('newRelease')}</Link>
        </div>
        {check && (
          <div>
            <p>
              {check.ok ? <span className="ok">ok</span> : <span className="err">{t('problems')}</span>}{' '}
              <span className="muted">{check.lang}</span>
            </p>
            {check.generate && <pre className="explain">{check.generate}</pre>}
            {(check.problems ?? []).length > 0 && (
              <table className="list">
                <tbody>
                  {(check.problems ?? []).map((p, i) => (
                    <tr key={i}>
                      <td>{p.warning ? '⚠' : '✗'}</td>
                      <td>
                        <code>
                          {p.pack}:{p.path}
                        </code>
                      </td>
                      <td>{p.message}</td>
                      <td className="muted">{p.source}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            {(check.scenarios ?? []).map((s, i) => (
              <div key={i}>
                {s.ok ? <span className="ok">✓ </span> : <span className="err">✗ </span>}
                <span>{s.name}</span>
                {(s.failures ?? []).map((f, j) => (
                  <pre key={j} className="explain">
                    {f}
                  </pre>
                ))}
              </div>
            ))}
          </div>
        )}
        {docsText && <textarea className="yaml" rows={16} value={docsText} readOnly />}
      </div>

      <div className="card">
        <h2>{t('editor')}</h2>
        <div className="row">
          <input value={filePath} placeholder="packs/kv/kv.go" onChange={(e) => setFilePath(e.target.value)} />
          <button disabled={!filePath} onClick={loadFile}>
            {t('load')}
          </button>
          <button disabled={!fileSHA} onClick={saveFile}>
            {t('save')}
          </button>
          {fileStatus && <span className="ok">{fileStatus}</span>}
        </div>
        {fileSHA && <textarea className="yaml" rows={16} value={fileContent} onChange={(e) => setFileContent(e.target.value)} />}
      </div>

      <div className="card">
        <h2>CEL</h2>
        <div className="row">
          <input value={expr} placeholder="values.port > 1024 ? values.port : 1025" onChange={(e) => setExpr(e.target.value)} />
          <button disabled={!expr || !packPath} onClick={evalCEL}>
            {t('run')}
          </button>
        </div>
        <div className="row">
          <textarea className="yaml" rows={3} placeholder='values (JSON): {"port": 5432}' value={celValues} onChange={(e) => setCelValues(e.target.value)} />
          <textarea className="yaml" rows={3} placeholder='facts (JSON): {"hosts": …}' value={celFacts} onChange={(e) => setCelFacts(e.target.value)} />
        </div>
        {celOut && <pre className="explain">{celOut}</pre>}
      </div>

      <div className="card">
        <h2>ref · example</h2>
        <div className="row">
          <select value={topic} onChange={(e) => setTopic(e.target.value)}>
            {['struct', 'root', 'cel', 'suggest', 'diagnose', 'action', 'files', 'packages', 'docs', 'holds', 'generate', 'test'].map((tp) => (
              <option key={tp}>{tp}</option>
            ))}
          </select>
          <button onClick={showRef}>ref</button>
          <select value={example} onChange={(e) => setExample(e.target.value)}>
            <option>postgres</option>
            <option>openbao</option>
            <option>docker</option>
          </select>
          <button onClick={showExample}>example</button>
        </div>
        {refText && <pre className="explain">{refText}</pre>}
        {Object.entries(exampleFiles).map(([name, content]) => (
          <details key={name}>
            <summary>
              <code>{name}</code>
            </summary>
            <pre className="explain">{content}</pre>
          </details>
        ))}
      </div>
    </>
  )
}
