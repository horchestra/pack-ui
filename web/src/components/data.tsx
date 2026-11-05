import { useEffect, useState, type ReactNode } from 'react'
import { ApiError, get, opGet } from '../api'
import { useT } from '../i18n'

// useData loads a document once (reload bumps it); the operator's paths go through the proxy.
export function useData<T>(path: string | null, reload?: number, op = true) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<ApiError | null>(null)
  const [loading, setLoading] = useState(!!path)
  useEffect(() => {
    if (!path) return
    let live = true
    setLoading(true)
    setError(null)
    ;(op ? opGet<T>(path) : get<T>('/api' + path))
      .then((d) => live && setData(d))
      .catch((e) => live && setError(e instanceof ApiError ? e : new ApiError(0, 'error', String(e))))
      .finally(() => live && setLoading(false))
    return () => {
      live = false
    }
  }, [path, reload, op])
  return { data, error, loading }
}

// Failure renders an API error as the page's error box; nothing installed yet (a 409 with pack's own
// document) is the document, not an error — the pages decide with it.
export function Failure({ error }: { error: ApiError | null }) {
  const t = useT()
  if (!error) return null
  return (
    <div className="failure" role="alert">
      {t('error')}: {error.code} — {error.message}
    </div>
  )
}

// Load shows the loading line, the error, or the children with the data.
export function Load<T>(props: { state: { data: T | null; error: ApiError | null; loading: boolean }; children: (d: T) => ReactNode }) {
  const t = useT()
  const { data, error, loading } = props.state
  if (loading) return <p className="muted">{t('loading')}</p>
  if (error) return <Failure error={error} />
  if (data == null) return null
  return <>{props.children(data)}</>
}
