import { Link } from 'react-router-dom'
import type { Job } from '../api'
import { Badge } from '../components/Badge'
import { Load, useData } from '../components/data'
import { useT } from '../i18n'

// The jobs, newest first; a state filter.

export function Jobs() {
  const t = useT()
  const state = useData<Job[]>('/jobs')
  return (
    <>
      <h1>{t('jobs')}</h1>
      <Load state={state}>
        {(jobs) => (
          <table className="list">
            <thead>
              <tr>
                <th>{t('job')}</th>
                <th>argv</th>
                <th>{t('state')}</th>
                <th>{t('user')}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((j) => (
                <tr key={j.id}>
                  <td>
                    <Link to={`/jobs/${j.id}`}>
                      <code>{j.id.slice(0, 12)}</code>
                    </Link>
                  </td>
                  <td>
                    <code>{(j.argv ?? []).slice(0, 6).join(' ')}</code>
                  </td>
                  <td>
                    <Badge value={j.state} />
                  </td>
                  <td>{j.worker ?? ''}</td>
                  <td>{j.created ? new Date(j.created).toLocaleString() : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Load>
    </>
  )
}
