// Package authn is who a call to pack-ui is (C9): the sessions of the SPA (an httpOnly cookie, a CSRF
// token of the session against a forged site), kept in memory — a restart logs everyone out. The local
// users prove a bcrypt password; the OIDC ones the provider's code flow (the operator's authn.OIDC, the
// groups are the operator's to weigh — pack-ui only learns the name). pack-ui holds no rights of its
// own: the operator's policy decides, the session's user goes to it as X-Pack-User with pack-ui's
// service token.
package authn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	opauthn "github.com/horchestra/pack/operator/authn"
	opconfig "github.com/horchestra/pack/operator/config"

	"github.com/horchestra/pack-ui/internal/config"
)

// Sessions is the open sessions: id → who and its CSRF token.
type Sessions struct {
	mu   sync.Mutex
	byID map[string]*session
}

type session struct {
	user    string
	csrf    string
	expires time.Time
}

// TTL is how long a login holds.
const TTL = 24 * time.Hour

// NewSessions is an empty store.
func NewSessions() *Sessions { return &Sessions{byID: map[string]*session{}} }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Open is a session for user: its id (the cookie) and its CSRF token.
func (s *Sessions) Open(user string) (id, csrf string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, csrf = randHex(32), randHex(32)
	s.byID[id] = &session{user: user, csrf: csrf, expires: time.Now().Add(TTL)}
	return id, csrf
}

// Get is who the session id is and its CSRF token; an unknown or expired one is none.
func (s *Sessions) Get(id string) (user, csrf string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.byID[id]
	if !ok || time.Now().After(se.expires) {
		delete(s.byID, id)
		return "", "", false
	}
	return se.user, se.csrf, true
}

// Drop ends the session.
func (s *Sessions) Drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

// LocalUser is the password checked against the configured users' hashes: the user, or "".
func LocalUser(users []config.User, name, password string) string {
	for _, u := range users {
		if u.Name == name && bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil {
			return name
		}
	}
	return ""
}

// NewOIDC is pack-ui's OIDC flow on its configuration (the operator's authn works it; no groups here —
// the operator maps them to roles on its own side).
func NewOIDC(ctx context.Context, cfg config.OIDC) (*opauthn.OIDC, error) {
	return opauthn.NewOIDC(ctx, opconfig.OIDC{
		Issuer:        cfg.Issuer,
		ClientID:      cfg.ClientID,
		ClientSecret:  cfg.ClientSecret,
		RedirectURL:   cfg.RedirectURL,
		UsernameClaim: cfg.UsernameClaim,
		GroupsClaim:   cfg.GroupsClaim,
	})
}
