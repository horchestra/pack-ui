import { Link } from 'react-router-dom'
import { opGet, relID, type Project, type ReleaseInfo } from '../api'
import { Badge } from '../components/Badge'
import { Failure, useData } from '../components/data'
import { useT } from '../i18n'
import { useEffect, useState } from 'react'

// The projects and their releases: each project's releases with the state next computes, the health, the
// last job.

export function ReleasesTable({ project, releases }: { project: string; releases: ReleaseInfo[] }) {
  const t = useT()
  return (
    <table className="list">
      <thead>
        <tr>
          <th>{t('release')}</th>
          <th>{t('state')}</th>
          <th>{t('next')}</th>
          <th>{t('health')}</th>
          <th>{t('lastJob')}</th>
        </tr>
      </thead>
      <tbody>
        {releases.map((r) => (
          <tr key={r.path}>
            <td>
              <Link to={`/r/${relID(project, r.path)}`}>{r.path}</Link>
            </td>
            <td>
              <Badge value={r.state} />
            </td>
            <td>{r.next ?? '—'}</td>
            <td>
              <Badge value={r.health} />
            </td>
            <td>
              {r.last_job ? (
                <Link to={`/jobs/${r.last_job.id}`}>
                  <Badge value={r.last_job.state} />
                </Link>
              ) : (
                '—'
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function ProjectReleases({ project }: { project: Project }) {
  const t = useT()
  const state = useData<ReleaseInfo[]>(`/projects/${project.id}/releases`)
  return (
    <section className="card" aria-label={project.id}>
      <h2>{project.id}</h2>
      {state.loading ? (
        <p className="muted">{t('loading')}</p>
      ) : state.error ? (
        <Failure error={state.error} />
      ) : (state.data ?? []).length === 0 ? (
        <p className="muted">{t('empty')}</p>
      ) : (
        <ReleasesTable project={project.id} releases={state.data ?? []} />
      )}
    </section>
  )
}

export function Projects() {
  const t = useT()
  // the projects pack-ui edits; the operator's list is the same id-space, and its read proves the operator
  const mine = useData<Project[]>('/projects', undefined, false)
  const [opOK, setOpOK] = useState<boolean | null>(null)
  useEffect(() => {
    opGet<unknown>('/version').then(
      () => setOpOK(true),
      () => setOpOK(false),
    )
  }, [])
  return (
    <>
      <h1>{t('projects')}</h1>
      {opOK === false && (
        <div className="failure" role="alert">
          the operator does not answer
        </div>
      )}
      {(mine.data ?? []).map((p) => (
        <ProjectReleases key={p.id} project={p} />
      ))}
      {mine.data?.length === 0 && <p className="muted">{t('empty')}</p>}
    </>
  )
}
