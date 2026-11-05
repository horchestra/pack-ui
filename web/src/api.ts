// The API the SPA talks to: pack-ui's own (/api/…) and the operator's through the proxy (/api/op/…, К4).
// A non-2xx comes back as an ApiError with the Failure's code.

export class ApiError extends Error {
  code: string
  status: number
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (resp.status >= 400) {
    let code = 'error'
    let message = resp.statusText
    try {
      const doc = await resp.json()
      if (doc?.error?.code) {
        code = doc.error.code
        message = doc.error.message || message
      }
    } catch {
      // not a Failure document: the status says it
    }
    throw new ApiError(resp.status, code, message)
  }
  return (await resp.json()) as T
}

export const get = <T>(path: string) => call<T>('GET', path)
export const post = <T>(path: string, body?: unknown) => call<T>('POST', path, body)
export const put = <T>(path: string, body?: unknown) => call<T>('PUT', path, body)

// The operator, through pack-ui's proxy.
export const opGet = <T>(path: string) => get<T>('/api/op' + path)
export const opPost = <T>(path: string, body?: unknown) => post<T>('/api/op' + path, body)

// A release is <project>:<path> in the operator's API (К4), URL-encoded — one path segment, the slashes
// of the path inside it.
export const relID = (project: string, path: string) => encodeURIComponent(`${project}:${path}`)

// relURL encodes a release id the router gave back decoded (useParams) for the API's path.
export const relURL = (rel: string) => encodeURIComponent(rel)

export interface Project {
  id: string
  repo: string
  branch: string
  flow: string
  runtime?: string
}

export interface JobBrief {
  id: string
  state: string
  how?: string
}

export interface ReleaseInfo {
  path: string
  next?: string
  sha: string
  installed_sha?: string
  state: string
  health?: string
  last_job?: JobBrief
}

export interface PackRequest {
  id: string
  key: string
  repo: string
  sha: string
  target: { release?: string; pack?: string }
  action: string
  params: Record<string, unknown>
  by: string
  at: string
  state: string
}

export interface ReleaseDetail extends ReleaseInfo {
  waiting: PackRequest[]
}

export interface Job {
  id: string
  request: string
  argv: string[]
  runtime: string
  state: string
  worker?: string
  saga?: string
  last_seq?: number
  result?: unknown
  exit?: number
  created?: string
  started?: string
  ended?: string
}

export interface RequestDetail extends PackRequest {
  digest: string
  jobs: Job[]
}

// A pack event (pkg/api.Event): a line of a job's or a saga's stream.
export interface PackEvent {
  seq: number
  time: string
  type: string // plan | node | log | saga | result
  key?: string
  step?: string
  status?: string
  message?: string
  error?: string
  saga?: string
  plan?: Plan
  result?: unknown
  code?: string
  next?: string[]
}

// A plan (pkg/plan.Plan), as a job's plan event carries it.
export interface Plan {
  pack: string
  from?: string
  to: string
  direction: string
  order: string[]
  changes: Change[]
  steps: Step[]
  units: Unit[]
  risks?: string[]
  digest?: string
}

export interface Change {
  key: string
  component: string
  from?: string
  to?: string
  kind: string // none | image | schema | config | drift | added | removed
  direction?: string
}

export interface Step {
  n: number
  key: string
  kind: string
  name?: string
  detail?: string
  phase?: string
  timeout?: string
  compensate?: string
  irreversible?: boolean
  target?: string
}

export interface Unit {
  savepoint: string
  from: number
  to: number
  keys: string[]
}

export interface GraphNode {
  name: string
  savepoint?: boolean
  waits?: string[]
}

export interface GraphResult {
  nodes: GraphNode[]
}

// A rendered file of pack template --json; a secret is only named.
export interface TemplateFile {
  key: string
  instance?: number
  path: string
  content?: string
  truncated?: boolean
  secret?: string
}

export interface TemplateResult {
  files: TemplateFile[]
}

export interface LogLine {
  path: string
  ref: string
  sha: string
  content: string
}

// The SSE stream of a job (К4): each event's id is its seq; Last-Event-ID resumes.
export function jobEvents(id: string, onEvent: (ev: PackEvent) => void, onError?: (e: Event) => void): () => void {
  const es = new EventSource(`/api/op/jobs/${encodeURIComponent(id)}/events`)
  es.onmessage = (m) => {
    try {
      onEvent(JSON.parse(m.data) as PackEvent)
    } catch {
      // a line that is no event is skipped
    }
  }
  if (onError) es.onerror = onError
  return () => es.close()
}
