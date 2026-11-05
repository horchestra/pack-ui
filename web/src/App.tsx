import { HashRouter, Link, Route, Routes } from 'react-router-dom'
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

// The shell: the header with the language and the theme, the routes. HashRouter — the SPA is served from a
// directory, a hash keeps the backend's fallback to index.html out of the way.

export function App() {
  const t = useT()
  const { lang, setLang } = useLang()
  const { theme, setTheme } = useTheme()
  return (
    <HashRouter>
      <header className="top">
        <span className="brand">pack-ui</span>
        <nav aria-label="main">
          <Link to="/">{t('projects')}</Link>
          <Link to="/jobs">{t('jobs')}</Link>
        </nav>
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
        <Routes>
          <Route path="/" element={<Projects />} />
          <Route path="/r/:rel" element={<Release />} />
          <Route path="/r/:rel/doc/:doc" element={<Docs />} />
          <Route path="/r/:rel/values" element={<Values />} />
          <Route path="/r/:rel/env" element={<Env />} />
          <Route path="/requests/:id" element={<RequestPage />} />
          <Route path="/jobs" element={<Jobs />} />
          <Route path="/jobs/:id" element={<JobPage />} />
        </Routes>
      </main>
    </HashRouter>
  )
}
