import { useEffect, useState } from 'react'
import { HashRouter, Link, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { get, post, setCSRF } from './api'
import { useT, useLang } from './i18n'
import { useTheme } from './theme'
import { Projects } from './pages/Projects'
import { Release } from './pages/Release'
import { RequestPage } from './pages/RequestPage'
import { JobPage } from './pages/JobPage'
import { Jobs } from './pages/Jobs'
import { Docs } from './pages/Docs'
import { Values } from './pages/Values'
import { Env } from './pages/Env'
import { Changes } from './pages/Changes'
import { ChangePage } from './pages/ChangePage'
import { NewRelease } from './pages/NewRelease'
import { Author } from './pages/Author'
import { Audit, Secrets } from './pages/Secrets'
import { Login } from './pages/Login'

// The shell: the header with the language and the theme, the routes. HashRouter — the SPA is served from a
// directory, a hash keeps the backend's fallback to index.html out of the way. Who the session is comes
// from whoami (C9): with an auth on and nobody logged in, the login is all there is.

interface Who {
  user: string
  auth: string
  csrf?: string
}

export function App() {
  const t = useT()
  const { lang, setLang } = useLang()
  const { theme, setTheme } = useTheme()
  const [who, setWho] = useState<Who | null>(null)

  useEffect(() => {
    get<Who>('/api/whoami').then((w) => {
      setCSRF(w.csrf ?? '')
      setWho(w)
    })
  }, [])

  const logout = () =>
    post('/api/logout').then(() => {
      setCSRF('')
      window.location.reload()
    })

  const needLogin = who !== null && who.auth !== 'none' && !who.user

  return (
    <HashRouter>
      <header className="top">
        <span className="brand">pack-ui</span>
        <nav aria-label="main">
          <Link to="/">{t('projects')}</Link>
          <Link to="/jobs">{t('jobs')}</Link>
          <Link to="/secrets">{t('secrets')}</Link>
          <Link to="/audit">{t('audit')}</Link>
        </nav>
        {who?.user && who.auth !== 'none' && <span className="muted">{who.user}</span>}
        {who?.user && who.auth !== 'none' && <button onClick={logout}>{t('logout')}</button>}
        <button
          aria-label={t('theme')}
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          title={theme === 'dark' ? t('light') : t('dark')}
        >
          {theme === 'dark' ? '☀' : '☾'}
        </button>
        <button onClick={() => setLang(lang === 'en' ? 'ru' : 'en')}>{t('language')}</button>
      </header>
      <main>
        {who === null ? null : (
          <Routes>
            <Route path="/login" element={needLogin ? <Login auth={who.auth} /> : <Navigate to="/" />} />
            <Route path="/" element={<Projects />} />
            <Route path="/r/:rel" element={<Release />} />
            <Route path="/r/:rel/doc/:doc" element={<Docs />} />
            <Route path="/r/:rel/values" element={<Values />} />
            <Route path="/r/:rel/env" element={<Env />} />
            <Route path="/p/:project/changes" element={<Changes />} />
            <Route path="/p/:project/new-release" element={<NewRelease />} />
            <Route path="/p/:project/author" element={<Author />} />
            <Route path="/changes/:id" element={<ChangePage />} />
            <Route path="/requests/:id" element={<RequestPage />} />
            <Route path="/jobs" element={<Jobs />} />
            <Route path="/jobs/:id" element={<JobPage />} />
            <Route path="/secrets" element={<Secrets />} />
            <Route path="/audit" element={<Audit />} />
          </Routes>
        )}
        {needLogin && <RedirectToLogin />}
      </main>
    </HashRouter>
  )
}

// RedirectToLogin pushes the login route wherever the SPA is (a 401 of a call does the same).
function RedirectToLogin() {
  const loc = useLocation()
  if (loc.pathname === '/login') return null
  return <Navigate to="/login" replace />
}
