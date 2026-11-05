package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	packdocs "github.com/horchestra/pack/docs"
	packexamples "github.com/horchestra/pack/examples"
	"github.com/horchestra/pack/pkg/bundle"
	"github.com/horchestra/pack/pkg/render"
	"github.com/horchestra/pack/pkg/scenario"
	"github.com/horchestra/pack/sdk/check"
	"github.com/horchestra/pack/sdk/docs"
	"github.com/horchestra/pack/sdk/format"
	"github.com/horchestra/pack/sdk/schema"

	"github.com/horchestra/pack-ui/internal/author"
	"github.com/horchestra/pack-ui/internal/values"
)

// The author's work (C6): scaffold a pack from a spec into the repository, check a pack's source as
// pack author check does (the generation, the checks, the docs, the scenarios on the memory target), the
// docs skeleton, the CEL sandbox, the ref and the examples. It all runs here, not through the operator —
// none of it is a deploy.

// scaffoldSchema answers GET /api/author/scaffold/schema — the JSON Schema a spec is constrained by.
func (s *Server) scaffoldSchema(w http.ResponseWriter, r *http.Request) {
	raw, err := author.SpecSchema()
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// scaffoldBody is the body of POST /api/repo/{id}/author/scaffold.
type scaffoldBody struct {
	Spec    json.RawMessage `json:"spec"` // the spec, JSON
	Lang    string          `json:"lang"` // go | python | java
	Dir     string          `json:"dir"`  // where the source goes in the repository (packs/<name>)
	Message string          `json:"message,omitempty"`
	Branch  string          `json:"branch,omitempty"`
}

// authorScaffold answers POST: the source of the spec written in the checkout, committed as the user, the
// operator synced.
func (s *Server) authorScaffold(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body scaffoldBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the body: %v", err)
		return
	}
	if len(body.Spec) == 0 || len(body.Lang) == 0 || len(body.Dir) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "spec, lang and dir are required")
		return
	}
	if strings.Contains(body.Dir, "..") {
		fail(w, http.StatusBadRequest, "bad_request", "the dir leaves the repository: %q", body.Dir)
		return
	}
	spec := &author.Spec{}
	if err := yaml.UnmarshalStrict(body.Spec, spec); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the spec: %v", err)
		return
	}
	if err := spec.Check(); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "the spec: %v", err)
		return
	}
	branch := p.Branch
	if p.Flow == "review" {
		if len(body.Branch) == 0 {
			fail(w, http.StatusBadRequest, "bad_request", "the project %s is flow: review — name the change request's branch", p.ID)
			return
		}
		branch = body.Branch
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	dir, err := os.MkdirTemp("", "pack-ui-scaffold-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	sha, err := repo.Checkout(r.Context(), branch, dir)
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return
	}
	out := filepath.Join(dir, filepath.FromSlash(body.Dir))
	// a checkout is no Go module: the import path of a Go source is the directory's, as the repository names it
	written, err := author.Scaffold(spec, body.Lang, out, author.ScaffoldOptions{GoImport: filepath.ToSlash(body.Dir)})
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	files := map[string][]byte{}
	for _, f := range written {
		raw, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			fail(w, http.StatusInternalServerError, "error", "%v", err)
			return
		}
		files[filepath.ToSlash(filepath.Join(filepath.FromSlash(body.Dir), f))] = raw
	}
	message := body.Message
	if len(message) == 0 {
		message = fmt.Sprintf("scaffold %s", body.Dir)
	}
	author_ := s.user(r) + " <" + s.user(r) + "@localhost>"
	landed, err := repo.Write(r.Context(), branch, sha, files, author_, message)
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	_ = s.opFor(r).Sync(r.Context(), p.ID, branch)
	answer(w, map[string]any{"sha": landed, "branch": branch, "files": len(files)})
}

// authorProblem is a check's finding, the source's file:line where it is found.
type authorProblem struct {
	Pack    string `json:"pack"`
	Path    string `json:"path"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
	Source  string `json:"source,omitempty"`
}

// authorChecked is the answer of POST /api/repo/{id}/author/check.
type authorChecked struct {
	OK        bool              `json:"ok"`
	Lang      string            `json:"lang"`
	Generate  string            `json:"generate,omitempty"`
	Problems  []authorProblem   `json:"problems,omitempty"`
	Scenarios []scenario.Result `json:"scenarios,omitempty"`
}

// generateFrom runs the generator of the source in src into a directory of its own (pack author check's).
func generateFrom(ctx context.Context, src string) (out, root, lang string, res authorChecked, err error) {
	if lang, err = author.Detect(src); err != nil {
		return "", "", "", res, err
	}
	res.Lang = lang
	if out, err = os.MkdirTemp("", "pack-author-"); err != nil {
		return "", "", "", res, err
	}
	err = author.Generate(ctx, src, lang, out, author.Options{})
	var ge *author.GenerateError
	if errors.As(err, &ge) {
		res.Generate = ge.Said
		return out, "", lang, res, nil
	}
	if err != nil {
		return out, "", lang, res, err
	}
	root, err = rootPack(out)
	return out, root, lang, res, err
}

// rootPack is the pack of the generated directory that no other holds.
func rootPack(out string) (string, error) {
	ents, err := os.ReadDir(out)
	if err != nil {
		return "", err
	}
	held := map[string]bool{}
	var names []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
		b, err := bundle.Load(filepath.Join(out, e.Name()))
		if err != nil {
			continue
		}
		for name := range b.Packs {
			if name != b.Name {
				held[name] = true
			}
		}
	}
	for _, n := range names {
		if !held[n] {
			return n, nil
		}
	}
	return "", fmt.Errorf("the generator wrote no pack under %s", out)
}

// checkDir is the author checks of a generated pack's directory (pack check's, without a release): the
// bundle's problems and the docs gaps.
func checkDir(dir string) ([]check.Problem, error) {
	b, err := bundle.Load(dir)
	if err != nil {
		return nil, err
	}
	problems, err := render.CheckBundle(b)
	if err != nil {
		return nil, err
	}
	for _, name := range slices.Sorted(maps.Keys(b.Packs)) {
		s, err := schema.Parse(b.Packs[name].SchemaRaw)
		if err != nil {
			continue // the checks above said it
		}
		for _, g := range docs.Gaps(s) {
			problems = append(problems, check.Problem{Path: name + ":" + g.Path, Message: g.Message, Warning: true})
		}
	}
	return problems, nil
}

// testsOf is the directory of scenarios of a source: tests/ beside it, else above it.
func testsOf(src string) string {
	for _, d := range []string{filepath.Join(src, "tests"), filepath.Join(filepath.Dir(src), "tests")} {
		if m, _ := filepath.Glob(filepath.Join(d, "*.yaml")); len(m) > 0 {
			return d
		}
	}
	return ""
}

// authorCheck answers POST /api/repo/{id}/author/check {path}: the source of the checkout's pack dir
// generated, checked, its docs and its scenarios — what to fix, with where.
func (s *Server) authorCheck(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body struct {
		Path   string `json:"path"`
		Branch string `json:"branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Path) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path is required")
		return
	}
	branch := p.Branch
	if len(body.Branch) > 0 {
		branch = body.Branch
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	dir, err := os.MkdirTemp("", "pack-ui-author-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	if _, err := repo.Checkout(r.Context(), branch, dir); err != nil {
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return
	}
	src := filepath.Join(dir, filepath.FromSlash(body.Path))
	out, root, lang, res, err := generateFrom(r.Context(), src)
	if len(out) > 0 {
		defer os.RemoveAll(out)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	if len(res.Generate) == 0 {
		names, _ := os.ReadDir(out)
		for _, e := range names {
			if !e.IsDir() {
				continue
			}
			ps, err := checkDir(filepath.Join(out, e.Name()))
			if err != nil {
				res.Problems = append(res.Problems, authorProblem{Pack: e.Name(), Path: "/", Message: err.Error()})
				continue
			}
			for _, pr := range ps {
				ap := authorProblem{Pack: e.Name(), Path: pr.Path, Message: pr.Message, Warning: pr.Warning}
				if where, ok := author.Locate(src, lang, pr.Path); ok {
					if rel, err := filepath.Rel(src, where.File); err == nil {
						where.File = rel
					}
					ap.Source = where.String()
				}
				res.Problems = append(res.Problems, ap)
			}
		}
		if tests := testsOf(src); len(tests) > 0 && len(root) > 0 {
			files, err := scenario.Files([]string{tests})
			if err != nil {
				fail(w, http.StatusBadRequest, "bad_request", "the scenarios: %v", err)
				return
			}
			on, err := scenario.MemoryStand(filepath.Join(out, root))
			if err != nil {
				fail(w, http.StatusInternalServerError, "error", "%v", err)
				return
			}
			for _, f := range files {
				res.Scenarios = append(res.Scenarios, scenario.RunScenario(r.Context(), on, f, false))
			}
		}
	}
	res.OK = len(res.Generate) == 0 && !slices.ContainsFunc(res.Problems, func(p authorProblem) bool { return !p.Warning }) &&
		!slices.ContainsFunc(res.Scenarios, func(sr scenario.Result) bool { return !sr.OK })
	answer(w, res)
}

// authorDocs answers POST /api/repo/{id}/author/docs {path, lang}: the docs/<lang>.yaml skeleton of the
// source's pack (pack docs skeleton).
func (s *Server) authorDocs(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body struct {
		Path   string `json:"path"`
		Lang   string `json:"lang"`
		Branch string `json:"branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Path) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path is required")
		return
	}
	branch := p.Branch
	if len(body.Branch) > 0 {
		branch = body.Branch
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	dir, err := os.MkdirTemp("", "pack-ui-author-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	if _, err := repo.Checkout(r.Context(), branch, dir); err != nil {
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return
	}
	out, root, _, res, err := generateFrom(r.Context(), filepath.Join(dir, filepath.FromSlash(body.Path)))
	if len(out) > 0 {
		defer os.RemoveAll(out)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	if len(res.Generate) > 0 {
		fail(w, http.StatusBadRequest, "check_failed", "the generator failed:\n%s", res.Generate)
		return
	}
	raw, err := os.ReadFile(filepath.Join(out, root, format.SchemaFile))
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	sc, err := schema.Parse(raw)
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	text := docs.Skeleton(sc, body.Lang)
	answer(w, map[string]string{"pack": root, "lang": body.Lang, "text": string(text)})
}

// authorCEL answers POST /api/repo/{id}/author/cel {path, key, expr, values, facts}: the expression worked
// out as a formula of the key would read it — on the values, the facts; the target a stub (pack-ui holds
// none), the limits of an environment left to the release's own checks.
func (s *Server) authorCEL(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	var body struct {
		Path   string         `json:"path"`
		Key    string         `json:"key,omitempty"`
		Expr   string         `json:"expr"`
		Values map[string]any `json:"values,omitempty"`
		Facts  map[string]any `json:"facts,omitempty"`
		Branch string         `json:"branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Expr) == 0 || len(body.Path) == 0 {
		fail(w, http.StatusBadRequest, "bad_request", "path and expr are required")
		return
	}
	branch := p.Branch
	if len(body.Branch) > 0 {
		branch = body.Branch
	}
	repo := s.repo(w, r, p)
	if repo == nil {
		return
	}
	dir, err := os.MkdirTemp("", "pack-ui-author-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	if _, err := repo.Checkout(r.Context(), branch, dir); err != nil {
		fail(w, http.StatusInternalServerError, "error", "the checkout: %v", err)
		return
	}
	out, root, _, res, err := generateFrom(r.Context(), filepath.Join(dir, filepath.FromSlash(body.Path)))
	if len(out) > 0 {
		defer os.RemoveAll(out)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	if len(res.Generate) > 0 {
		fail(w, http.StatusBadRequest, "check_failed", "the generator failed:\n%s", res.Generate)
		return
	}
	b, err := bundle.Load(filepath.Join(out, root))
	if err != nil {
		fail(w, http.StatusInternalServerError, "error", "%v", err)
		return
	}
	key := body.Key
	if len(key) == 0 {
		if len(b.Keys) != 1 {
			fail(w, http.StatusBadRequest, "bad_request", "key: the pack has the keys %s", strings.Join(b.Keys, ", "))
			return
		}
		key = b.Keys[0]
	}
	rc := render.Context{Installation: scenario.TestInstallation, Target: values.StubTarget{}}
	if len(body.Facts) > 0 {
		rc.Facts = map[string]map[string]any{"": body.Facts}
	}
	got, err := render.Eval(b, render.Values{Values: body.Values}, rc, key, body.Expr)
	if err != nil {
		fail(w, http.StatusBadRequest, "values_invalid", "%v", err)
		return
	}
	answer(w, map[string]any{"key": key, "value": got})
}

// refTopics are the topics of authoring.md (pack author ref's).
var refTopics = map[string]string{
	"struct":   "## The struct",
	"root":     "## PackRoot",
	"cel":      "## Formulas and rules",
	"suggest":  "A **suggestion**",
	"diagnose": "A **diagnosis**",
	"action":   "An **action**",
	"files":    "## Files",
	"packages": "## Installing from OS packages",
	"docs":     "## Documentation",
	"holds":    "## A pack that holds a pack",
	"generate": "## Generating",
	"test":     "## Testing",
}

var codeFence = regexp.MustCompile("^```(\\w*)")

// refOf is the topic's part of the guide, the code blocks of other languages left out (pack author ref's).
func refOf(guide, topic, lang string) (string, error) {
	start, ok := refTopics[topic]
	if !ok {
		return "", fmt.Errorf("topic %q: one of %s", topic, strings.Join(slices.Sorted(maps.Keys(refTopics)), ", "))
	}
	lines := strings.Split(guide, "\n")
	from := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, start) })
	if from < 0 {
		return "", fmt.Errorf("topic %q: not in the guide", topic)
	}
	heading := strings.HasPrefix(start, "#")
	var out []string
	skip, inCode := false, false
	for i := from; i < len(lines); i++ {
		l := lines[i]
		if i > from && !inCode {
			if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "# ") {
				break
			}
			if !heading && (strings.HasPrefix(l, "A **") || strings.HasPrefix(l, "An **") || strings.HasPrefix(l, "The expression sees")) {
				break
			}
		}
		if m := codeFence.FindStringSubmatch(l); m != nil {
			if !inCode {
				inCode = true
				code := m[1]
				skip = len(code) > 0 && code != lang && slices.Contains(author.Langs, code)
			} else {
				inCode = false
				if skip {
					skip = false
					continue
				}
			}
		}
		if !skip {
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n", nil
}

// authorRef answers GET /api/author/ref?topic=&lang= (pack author ref).
func (s *Server) authorRef(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if len(lang) == 0 {
		lang = "go"
	}
	text, err := refOf(packdocs.Authoring, r.URL.Query().Get("topic"), lang)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "%v", err)
		return
	}
	answer(w, map[string]string{"topic": r.URL.Query().Get("topic"), "text": text})
}

// authorExample answers GET /api/author/example?name=&lang= — an example pack's source in a language
// (pack author example).
func (s *Server) authorExample(w http.ResponseWriter, r *http.Request) {
	name, lang := r.URL.Query().Get("name"), r.URL.Query().Get("lang")
	if len(lang) == 0 {
		lang = "go"
	}
	dir := path.Join(name, lang)
	ents, err := fs.ReadDir(packexamples.Sources, dir)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "example %s in %s: there are postgres, openbao and docker, in go, python and java", name, lang)
		return
	}
	files := map[string]string{}
	for _, e := range ents {
		if !packexamples.IsSource(e.Name()) {
			continue
		}
		b, err := fs.ReadFile(packexamples.Sources, path.Join(dir, e.Name()))
		if err != nil {
			fail(w, http.StatusInternalServerError, "error", "%v", err)
			return
		}
		files[e.Name()] = string(b)
	}
	answer(w, map[string]any{"example": name, "lang": lang, "files": files})
}
