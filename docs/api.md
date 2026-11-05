# The HTTP API

pack-ui's backend answers JSON; every error is a **Failure document** — the shape the operator's API
writes (К4):

```json
{"error": {"code": "conflict", "message": "conflict: 9f2e1ab changed values.yaml too",
           "detail": {"files": ["values.yaml"], "their_sha": "…", "their_subject": "…"}}}
```

The codes a caller branches on: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`,
`foreign_comments`, `values_invalid`, `check_failed`, `error`.

Every call is behind the session middleware: with `auth` on, no session is a 401 `unauthorized` (except
`whoami`, the login's own endpoints, and the OIDC flow); a mutation with the session's cookie and no
`X-CSRF-Token` is a 403 `forbidden`; a bearer of `tokens:` passes without either.

## The session

| endpoint | what it is |
|---|---|
| `GET /api/whoami` | `{user, auth, csrf}` — who the requests are for (auth `none` — the configured user; not logged in — the empty user, the SPA shows its login) and the session's CSRF token |
| `POST /api/login` | `{user, password}` → the session's cookie and the CSRF token (auth `local`) |
| `POST /api/logout` | the session dropped, the cookie cleared |
| `GET /api/auth/oidc/login` | off to the provider (auth `oidc`) |
| `GET /api/auth/oidc/callback` | the id token says who; the groups granted as roles in every project; the session opens, the SPA is the redirect |

## The repositories

| endpoint | what it is |
|---|---|
| `GET /api/projects` | the projects pack-ui edits: `[{id, repo, branch, flow}]` |
| `GET /api/repo/{id}/file?path=&ref=` | `{path, ref, sha, content}` — the file at the branch's head |
| `PUT /api/repo/{id}/file` | `{path, content, message, sha?, branch?}` → `{sha, branch, synced}` ([editing.md](editing.md)) |
| `GET /api/repo/{id}/log?path=&ref=` | `[{sha, subject}]` — the commits that touched the path, newest first |
| `GET /api/repo/{id}/diff?base=&head=` | `[{path, from, to}]` — the change of `head` against the merge-base with `base` |
| `POST /api/repo/{id}/init` | a release made by `pack init` in a checkout, committed, synced, a `refresh` request opened — see below |

### init

```json
{"pack": "oci://registry.example.org/packs/postgres:1.2.0   — or packs/postgres, a directory of the repository",
 "path": "releases/prod/db", "target": "docker",
 "target_source": "…", "target_image": "…", "runtime": "…", "lang": "ru",
 "message": "…", "branch": "cr-12"}
```

Runs the configured `pack` binary's `init` in a checkout (no terminal — the target the form names),
commits the files it made as the user, pushes, syncs, waits until the operator knows the release (up to
30 s), and opens the `refresh` request that locks and fetches the pack. A pack of the repository is
recorded in `release.yaml` relative to the release, so the two move together. Answers
`{sha, branch, files, request}`.

## The values

[values.md](values.md): `POST /api/repo/{id}/values/check` (the live help), `/values/preview` (the file
and the foreign comments), `/values/write` (the commit; 409 `foreign_comments` unless `rewrite: true`).

## The authoring

[authoring.md](authoring.md): `GET /api/author/scaffold/schema`,
`POST /api/repo/{id}/author/{scaffold,check,docs,cel}`, `GET /api/author/ref`, `GET /api/author/example`.

## The diagnosis

[diagnosis.md](diagnosis.md): `POST /api/repo/{id}/diagnose`.

## The proxy

`ANY /api/op/*` → the operator's `/api/*`, as the session's user. The escaped path is kept (a release id is
one segment, its slashes `%2F`), the cookie is dropped, pack-ui's service token attached. SSE streams
through unbuffered; `Last-Event-ID` is a header like any other.

## The SPA

Everything that is not `/api/*`: a path that names a file is served; anything else falls to `index.html` —
the SPA routes. With no SPA at all (the embedded `dist` is the placeholder and no `static:` configured),
the API is all there is and `/` is a 404 that says so.
