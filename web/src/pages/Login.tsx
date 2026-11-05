import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { post, setCSRF } from '../api'
import { useT } from '../i18n'

// The login (C9): local — a name and a password; OIDC — off to the provider, the callback returns to the
// SPA. The session is an httpOnly cookie; the answer's CSRF token is what the mutations send.
export function Login({ auth }: { auth: string }) {
  const t = useT()
  const navigate = useNavigate()
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  const login = () => {
    setError('')
    post<{ user: string; csrf: string }>('/api/login', { user, password })
      .then((s) => {
        setCSRF(s.csrf)
        navigate('/')
        window.location.reload()
      })
      .catch((e) => setError(String(e)))
  }

  return (
    <div className="card" style={{ maxWidth: '24rem', margin: '4rem auto' }}>
      <h1>{t('login')}</h1>
      {auth === 'oidc' ? (
        <p>
          <a className="primary" href="/api/auth/oidc/login">
            {t('loginOIDC')}
          </a>
        </p>
      ) : (
        <>
          <p>
            <input
              placeholder={t('user')}
              value={user}
              autoFocus
              onChange={(e) => setUser(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && login()}
            />
          </p>
          <p>
            <input
              type="password"
              placeholder={t('password')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && login()}
            />
          </p>
          <p>
            <button className="primary" disabled={!user || !password} onClick={login}>
              {t('login')}
            </button>
          </p>
        </>
      )}
      {error && <p className="err">{error}</p>}
    </div>
  )
}
