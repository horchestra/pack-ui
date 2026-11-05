# The SPA (web/)

React 18, TypeScript, Vite, `react-router-dom` with a **HashRouter** — the SPA is served from a directory
and a hash keeps the backend's `index.html` fallback out of the way. `@rjsf` renders the values form from
a pack's JSON Schema, `mermaid` draws the plans and the graphs, `yaml` round-trips the values. The build
lands in `web/dist` and is embedded into the binary (`web/embed.go`) — pack-ui is one binary.

```sh
cd web
npm ci
npm run dev     # the Vite dev server, against a running backend
npm run build   # tsc -b && vite build — into dist/
npm test        # vitest + testing-library
```

For the backend to serve a fresh build without a rebuild of the binary, point `static:` at `web/dist`
([configuration.md](configuration.md)).

## The API layer (src/api.ts)

- `get` / `post` / `put` — pack-ui's own API; `opGet` / `opPost` — the operator's, through the proxy
  (`/api/op` + path). The browser knows pack-ui only.
- A non-2xx is an `ApiError` with the Failure's `code` — the pages branch on the code (`conflict`,
  `foreign_comments`), not on the message.
- A 401 anywhere redirects to `#/login`.
- The CSRF token `whoami` issued (`setCSRF`) rides as `X-CSRF-Token` on every mutation.
- A release id is `<project>:<path>`, URL-encoded into **one** path segment (`relID`) — the operator's
  router matches segments escaped.
- `jobEvents(id, onEvent)` is a job's SSE stream: each event's id is its `seq`, and the browser's
  EventSource resumes with `Last-Event-ID` on its own.

## The shell and the routes (src/App.tsx)

The header: the navigation, the language (en/ru — `src/i18n.tsx`), the theme (dark/light —
`src/theme.tsx`), and, with auth on, the user and the logout. Until `whoami` answers, nothing renders;
with auth on and nobody logged in, the login is all there is.

| route | page | what it is |
|---|---|---|
| `/` | Projects | the projects and their releases |
| `/r/:rel` | Release | a release: the status, the history, the graph, the template, the actions — through the proxy |
| `/r/:rel/doc/:doc` | Docs | a pack's rendered docs |
| `/r/:rel/values` | Values | the schema form: the live check, the preview, the write ([values.md](values.md)) |
| `/r/:rel/env` | Env | the release's `env.yaml` |
| `/p/:project/changes` | Changes | the change requests of a `flow: review` project |
| `/changes/:id` | ChangePage | a change request: its diff (`GET …/diff`), the merge and the close are the operator's |
| `/p/:project/new-release` | NewRelease | the init form (`POST …/init`) |
| `/p/:project/author` | Author | scaffold, check, docs, the CEL sandbox, the ref, the examples ([authoring.md](authoring.md)) |
| `/requests/:id` | RequestPage | a deploy request: the plan, the digest, the approval — a person's, always |
| `/jobs`, `/jobs/:id` | Jobs, JobPage | the jobs and their live event streams |
| `/secrets` | Secrets | the names and the keys — never a value |
| `/audit` | Audit | the operator's audit |
| `/login` | Login | the local password or off to the OIDC provider |

The components (`src/components/`): `Diff` (the change request's files), `EventFeed` (a job's or saga's
stream), `Json`, `Keys`, `Mermaid`, `Badge`, `data.tsx` (the operator's documents as types).

## Tests

- `npm test` — vitest: the components and the values form (`src/test/`).
- `e2e/` — Playwright, a module of its own. The Go harness builds the whole system (the operator, pack-ui,
  a git remote) and gives `PACK_UI_URL`; one worker — the operator serializes the deploy itself. The
  system's Google Chrome is used, no browser download.
