// Package config is the configuration of pack-ui (К4, К5): where the operator's API is, who the user is until
// authentication lands (auth: none, the user goes to the operator as X-Pack-User), how pack-ui signs its
// commits (the committer; the author is the user), where the working copies of the projects' repositories
// live, and the projects themselves — a mirror of the operator's, a project is edited only when both know it.
package config

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

// Config is the file pack-ui runs on.
type Config struct {
	Listen        string    `json:"listen"`         // :8080
	Operator      string    `json:"operator"`       // the operator API's base URL
	OperatorToken string    `json:"operator_token"` // C9: the bearer pack-ui proves itself to the operator with (auth local|oidc)
	Auth          string    `json:"auth"`           // none | local | oidc (C9)
	User          string    `json:"user"`           // auth: none — who the requests are for (X-Pack-User)
	Users         []User    `json:"users"`          // auth: local
	Tokens        []Token   `json:"tokens"`         // the CI tokens of pack-ui itself (a bearer, no session, no CSRF)
	OIDC          OIDC      `json:"oidc"`           // auth: oidc
	Static        string    `json:"static"`         // the SPA's directory; empty — the one embedded into the binary (web/dist)
	Repos         string    `json:"repos"`          // the working copies' directory
	Pack          string    `json:"pack"`           // the pack binary a release's init runs (C5)
	Secrets       Secrets   `json:"secrets"`
	Git           Git       `json:"git"`
	Projects      []Project `json:"projects"`
}

// User is a local user (auth: local): the name and the bcrypt hash of the password. What the user may is
// the operator's policy — pack-ui holds no rights of its own.
type User struct {
	Name         string `json:"name"`
	PasswordHash string `json:"password_hash"` // bcrypt
}

// Token is a CI token of pack-ui itself: the bearer is the secret, the calls are the user's. (The
// operator's own tokens are its /api/tokens; these are pack-ui's door for a model or a pipeline.)
type Token struct {
	Name  string `json:"name"`
	Token string `json:"token"`
	User  string `json:"user"`
}

// OIDC is the OpenID Connect login (auth: oidc), the same flow the operator has. The groups map to the
// roles of К7: at login they are granted in every project through the operator's /api/grants (pack-ui's
// service token is an admin's).
type OIDC struct {
	Issuer        string            `json:"issuer"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"client_secret"`
	RedirectURL   string            `json:"redirect_url"`
	UsernameClaim string            `json:"username_claim"` // preferred_username by default
	GroupsClaim   string            `json:"groups_claim"`   // groups by default
	Groups        map[string]string `json:"groups"`         // the provider's group → the role
}

// Secrets is where the git credentials are read from — the same store the operator reads (К5): OpenBao.
type Secrets struct {
	Provider string `json:"provider"` // openbao
	Addr     string `json:"addr"`
	Mount    string `json:"mount"`
	TokenEnv string `json:"token_env"`
}

// Git is how pack-ui commits.
type Git struct {
	Committer string `json:"committer"` // "pack-ui <pack-ui@example.org>" (К5)
}

// Project is a repository pack-ui edits, a mirror of the operator's project of the same id.
type Project struct {
	ID          string `json:"id"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`      // the main branch; default main
	Flow        string `json:"flow"`        // direct — push to the main branch; review — to a change request's branch
	Credentials string `json:"credentials"` // the reference in the secrets store
}

// Load reads the file, fills the defaults, refuses what would not run.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(c.Listen) == 0 {
		c.Listen = ":8080"
	}
	if len(c.Operator) == 0 {
		return nil, fmt.Errorf("%s: operator is required — the API pack-ui talks to", path)
	}
	c.Operator = strings.TrimSuffix(c.Operator, "/")
	if len(c.Auth) == 0 {
		c.Auth = "none"
	}
	switch c.Auth {
	case "none":
	case "local":
		if len(c.Users) == 0 {
			return nil, fmt.Errorf("%s: auth local needs users (a name and the bcrypt password_hash)", path)
		}
		for i, u := range c.Users {
			if len(u.Name) == 0 || len(u.PasswordHash) == 0 {
				return nil, fmt.Errorf("%s: users[%d]: name and password_hash are required", path, i)
			}
		}
	case "oidc":
		for _, f := range []struct{ name, v string }{
			{"issuer", c.OIDC.Issuer}, {"client_id", c.OIDC.ClientID}, {"client_secret", c.OIDC.ClientSecret}, {"redirect_url", c.OIDC.RedirectURL},
		} {
			if len(f.v) == 0 {
				return nil, fmt.Errorf("%s: auth oidc needs oidc.%s", path, f.name)
			}
		}
	default:
		return nil, fmt.Errorf("%s: auth is none, local or oidc, not %q", path, c.Auth)
	}
	if c.Auth != "none" && len(c.OperatorToken) == 0 {
		return nil, fmt.Errorf("%s: auth %s needs operator_token — the bearer pack-ui proves itself to the operator with (its user an admin, X-Pack-User acts as the session's)", path, c.Auth)
	}
	if len(c.User) == 0 {
		c.User = "dev"
	}
	if len(c.Repos) == 0 {
		return nil, fmt.Errorf("%s: repos is required — where the working copies live", path)
	}
	if len(c.Pack) == 0 {
		c.Pack = "pack"
	}
	if len(c.Git.Committer) == 0 {
		c.Git.Committer = "pack-ui <pack-ui@localhost>"
	}
	if !strings.Contains(c.Git.Committer, " <") || !strings.HasSuffix(c.Git.Committer, ">") {
		return nil, fmt.Errorf("%s: git.committer is \"Name <address>\": %q", path, c.Git.Committer)
	}
	if c.Secrets.Provider != "openbao" && len(c.Secrets.Provider) > 0 {
		return nil, fmt.Errorf("%s: secrets.provider %q: only \"openbao\"", path, c.Secrets.Provider)
	}
	ids := map[string]bool{}
	for i := range c.Projects {
		p := &c.Projects[i]
		if len(p.ID) == 0 || len(p.Repo) == 0 {
			return nil, fmt.Errorf("%s: a project needs id and repo", path)
		}
		if len(p.Branch) == 0 {
			p.Branch = "main"
		}
		if len(p.Flow) == 0 {
			p.Flow = "direct"
		}
		if p.Flow != "direct" && p.Flow != "review" {
			return nil, fmt.Errorf("%s: project %s: flow is direct or review: %q", path, p.ID, p.Flow)
		}
		if ids[p.ID] {
			return nil, fmt.Errorf("%s: the project %s twice", path, p.ID)
		}
		ids[p.ID] = true
	}
	return &c, nil
}

// Project is the project of id, or nil.
func (c *Config) Project(id string) *Project {
	for i := range c.Projects {
		if c.Projects[i].ID == id {
			return &c.Projects[i]
		}
	}
	return nil
}
