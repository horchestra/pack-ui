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
	Listen   string   `json:"listen"`   // :8080
	Operator string   `json:"operator"` // the operator API's base URL
	Auth     string   `json:"auth"`     // none — until C9; anything else is refused for now
	User     string   `json:"user"`     // auth: none — who the requests are for (X-Pack-User)
	Static   string   `json:"static"`   // the SPA's directory; empty — the API only
	Repos    string   `json:"repos"`    // the working copies' directory
	Secrets  Secrets  `json:"secrets"`
	Git      Git      `json:"git"`
	Projects []Project `json:"projects"`
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
	if c.Auth != "none" {
		return nil, fmt.Errorf("%s: auth %q: only \"none\" exists until C9", path, c.Auth)
	}
	if len(c.User) == 0 {
		c.User = "dev"
	}
	if len(c.Repos) == 0 {
		return nil, fmt.Errorf("%s: repos is required — where the working copies live", path)
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
