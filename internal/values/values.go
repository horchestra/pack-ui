// Package values is the values of a release as pack-ui writes them (C3): the schema's form checked on the
// fly and guided by what is missing (render.Ask — the fill pack does), the file rendered exactly as
// pack values write renders it (pkg/valuesfile) and committed. A pack in the repository only: a release
// whose pack is an OCI reference is rendered by the operator's checkout, not here — yet.
package values

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/horchestra/pack/pkg/bundle"
	"github.com/horchestra/pack/pkg/render"
	"github.com/horchestra/pack/pkg/valuesfile"
)

// Release is a release's directory in a checkout of the repository (the branch's head).
type Release struct {
	dir  string // the checkout
	rel  string // the release's path in it
	have []byte // the values.yaml it has now
	id   string // the installation id
}

// releaseDoc is what of release.yaml the values' work needs.
type releaseDoc struct {
	Pack string `json:"pack"`
	ID   string `json:"id"`
}

// Load is the release of rel in the checkout: its values.yaml as it is, its id. A file that is a whole
// document (values:, rollout:, skip:) is refused — a write would lose the rest (pack values write refuses
// the same).
func Load(checkout, rel string) (*Release, map[string]any, error) {
	r := &Release{dir: checkout, rel: rel}
	raw, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(rel), "release.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", rel, err)
	}
	var doc releaseDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", rel, err)
	}
	r.id = doc.ID
	if strings.Contains(doc.Pack, "://") {
		return nil, nil, fmt.Errorf("%s: the pack %s is a reference, not a directory in the repository — values of such a release are written where the pack is pulled", rel, doc.Pack)
	}
	r.have, err = os.ReadFile(r.valuesPath())
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	var values map[string]any
	if len(r.have) > 0 {
		var whole map[string]any
		if err := yaml.Unmarshal(r.have, &whole); err != nil {
			return nil, nil, fmt.Errorf("values.yaml: %w", err)
		}
		for _, key := range []string{"values", "rollout", "skip"} {
			if _, ok := whole[key]; ok {
				return nil, nil, fmt.Errorf("values.yaml: a whole document (%s:) is written by hand, not here", key)
			}
		}
		values = whole
	}
	return r, values, nil
}

func (r *Release) valuesPath() string {
	return filepath.Join(r.dir, filepath.FromSlash(r.rel), "values.yaml")
}

// bundle is the pack of the release.
func (r *Release) bundle() (*bundle.Bundle, error) {
	raw, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(r.rel), "release.yaml"))
	if err != nil {
		return nil, err
	}
	var doc releaseDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	pack := filepath.Clean(filepath.Join(r.dir, filepath.FromSlash(r.rel), filepath.FromSlash(doc.Pack)))
	if !strings.HasPrefix(pack, filepath.Clean(r.dir)+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s: the pack leaves the repository: %s", r.rel, doc.Pack)
	}
	return bundle.Load(pack)
}

// stubTarget is the render's target where the form is: pack-ui holds no targets (the worker's pack does),
// so a key resolves to its own name. A suggestion that reads a target's facts offers nothing here — the
// form's help is the schema's, not the environment's.
type stubTarget struct{}

func (stubTarget) Namespace() string { return "" }

func (stubTarget) Resolve(key string, _ render.Placement) (*render.Resolved, error) {
	return &render.Resolved{Service: key, Address: key}, nil
}

// Ask is the form's live help (the fill pack asks): the settings required and missing, the suggestions the
// pack makes for the values as the form holds them, the problems of it — every one where the schema points.
// The context is the release alone: the target is a stub, there are no facts.
func (r *Release) Ask(values map[string]any) (*render.Questions, error) {
	b, err := r.bundle()
	if err != nil {
		return nil, err
	}
	return render.Ask(b, render.Values{Values: values}, render.Context{Installation: r.id, Target: stubTarget{}})
}

// Render is the values.yaml of values exactly as pack values write would write it (pkg/valuesfile), and the
// comments of the operator's own the file has that the render would lose (Foreign — the caller refuses
// unless told to rewrite).
func (r *Release) Render(values map[string]any, lang string) (want []byte, foreign []string, err error) {
	b, err := r.bundle()
	if err != nil {
		return nil, nil, err
	}
	want, err = valuesfile.Write(b, values, valuesfile.Sources{}, valuesfile.Lang(lang))
	if err != nil {
		return nil, nil, err
	}
	if len(r.have) > 0 {
		foreign = valuesfile.Foreign(r.have, want)
	}
	return want, foreign, nil
}
