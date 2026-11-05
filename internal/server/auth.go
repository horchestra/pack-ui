package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/horchestra/pack-ui/internal/authn"
)

// The authentication of C9 in pack-ui: a login (local — a bcrypt password of the configured users; OIDC —
// the provider's code flow) opens a session, an httpOnly cookie; a mutation with the cookie needs the
// session's X-CSRF-Token. pack-ui holds no rights: the session's user goes to the operator as X-Pack-User
// with pack-ui's service token, and the operator's policy decides. auth: none is the configured user, as
// before.

// sessionCookie is the SPA's session.
const sessionCookie = "pack_ui_session"

// authn is the middleware of /api: the login's own endpoints stay open, everything else wants a session
// (or, with auth none, is the configured user); a mutating call with a session wants its X-CSRF-Token. The
// user lands in the request's context for the handlers and the proxy.
func (s *Server) authn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/api/login" || r.URL.Path == "/api/logout" || strings.HasPrefix(r.URL.Path, "/api/auth/oidc/") {
			next.ServeHTTP(w, r)
			return
		}
		user, csrf := s.sessionUser(r)
		if len(user) == 0 {
			// a CI bearer of the configuration's tokens: no session, no CSRF
			if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
				for _, tok := range s.cfg.Tokens {
					if tok.Token == strings.TrimPrefix(v, "Bearer ") {
						user = tok.User
						break
					}
				}
			}
		}
		if len(user) == 0 {
			if s.cfg.Auth == "none" {
				user = s.cfg.User
			} else if r.URL.Path == "/api/whoami" {
				answer(w, map[string]string{"user": "", "auth": s.cfg.Auth})
				return
			} else {
				fail(w, http.StatusUnauthorized, "unauthorized", "log in first (auth: %s)", s.cfg.Auth)
				return
			}
		}
		if len(csrf) > 0 && r.Method != http.MethodGet && r.Method != http.MethodHead &&
			r.Header.Get("X-CSRF-Token") != csrf {
			fail(w, http.StatusForbidden, "forbidden", "a mutation with the session's cookie needs its X-CSRF-Token")
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
	})
}

// sessionUser is who the session cookie is of, and the session's CSRF token; "" — no session.
func (s *Server) sessionUser(r *http.Request) (user, csrf string) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if u, tok, ok := s.sessions.Get(c.Value); ok {
			return u, tok
		}
	}
	return "", ""
}

// login answers POST /api/login {user, password} (auth: local): the session's cookie and the CSRF token.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth != "local" {
		fail(w, http.StatusNotFound, "not_found", "no local users with auth: %s", s.cfg.Auth)
		return
	}
	var body struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the body is {\"user\", \"password\"}")
		return
	}
	if user := authn.LocalUser(s.cfg.Users, body.User, body.Password); len(user) > 0 {
		s.openSession(w, r, user)
		return
	}
	fail(w, http.StatusUnauthorized, "unauthorized", "the user or the password is wrong")
}

// openSession is the session of a login, whichever way it proved itself.
func (s *Server) openSession(w http.ResponseWriter, r *http.Request, user string) {
	id, csrf := s.sessions.Open(user)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(authn.TTL), Secure: r.TLS != nil,
	})
	answer(w, map[string]string{"user": user, "csrf": csrf})
}

// logout answers POST /api/logout: the session dropped, the cookie cleared.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.Drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
	answer(w, map[string]string{"bye": "bye"})
}

// oidcLogin answers GET /api/auth/oidc/login: off to the provider.
func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		fail(w, http.StatusNotFound, "not_found", "no OIDC with auth: %s", s.cfg.Auth)
		return
	}
	s.oidc.Login(w, r)
}

// oidcCallback answers GET /api/auth/oidc/callback: the id token says who; the groups, mapped by the
// configuration, are granted in every project through the operator's /api/grants (pack-ui's token is an
// admin's); the session opens, the SPA is the redirect.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		fail(w, http.StatusNotFound, "not_found", "no OIDC with auth: %s", s.cfg.Auth)
		return
	}
	id, err := s.oidc.Callback(w, r)
	if err != nil {
		fail(w, http.StatusUnauthorized, "unauthorized", "%v", err)
		return
	}
	for _, g := range id.Groups {
		role, ok := s.cfg.OIDC.Groups[g]
		if !ok {
			continue
		}
		for _, p := range s.cfg.Projects {
			_ = s.op.Post(r.Context(), "/api/grants",
				map[string]string{"user": id.User, "role": role, "dom": p.ID}, nil)
		}
	}
	s.openSession(w, r, id.User)
}
