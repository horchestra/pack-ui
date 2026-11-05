# The configuration: pack-ui.yaml

pack-ui runs on one YAML file (`--config`, `pack-ui.yaml` by default). It is read once at start; what
would not run is refused with the file and the field named.

```yaml
listen: :8090
operator: http://operator:9090
operator_token: eyJ…

auth: local
user: dev
users:
  - { name: alice, password_hash: "$2a$10$…" }
tokens:
  - { name: ci, token: "s3cr3t", user: robot }

oidc:
  issuer: https://idp.example.org
  client_id: pack-ui
  client_secret: …
  redirect_url: https://ui.example.org/api/auth/oidc/callback
  username_claim: preferred_username
  groups_claim: groups
  groups: { pack-admins: admin, pack-devs: write }

static: ""
repos: /var/lib/pack-ui/repos
pack: /usr/local/bin/pack

secrets:
  provider: openbao
  addr: https://bao.example.org
  mount: secret
  token_env: BAO_TOKEN

git:
  committer: "pack-ui <pack-ui@example.org>"

projects:
  - id: dev
    repo: https://git.example.org/platform/dev.git
    branch: main
    flow: review
    credentials: git/dev
```

## Field by field

| field            | default                    | what it is                                                                                     |
|------------------|----------------------------|------------------------------------------------------------------------------------------------|
| `listen`         | `:8080`                    | where pack-ui listens                                                                           |
| `operator`       | — **required**             | the operator API's base URL                                                                     |
| `operator_token` | —                          | the bearer pack-ui proves itself to the operator with; required when `auth` is `local` or `oidc` |
| `auth`           | `none`                     | `none` \| `local` \| `oidc` — who a call is ([architecture.md](architecture.md))                 |
| `user`           | `dev`                      | `auth: none` — who every request is for; sent to the operator as `X-Pack-User`                   |
| `users`          | —                          | `auth: local` — the names and the bcrypt hashes of the passwords; at least one                   |
| `tokens`         | —                          | CI tokens of pack-ui itself: the bearer is the secret, the calls are its `user`'s; no session, no CSRF |
| `oidc`           | —                          | `auth: oidc` — the issuer, the client, the redirect; the claims' names default to `preferred_username` and `groups` |
| `oidc.groups`    | —                          | the provider's group → the operator's role; granted in every project at login                    |
| `static`         | the embedded `web/dist`    | the SPA's directory; empty — the one built into the binary                                       |
| `repos`          | — **required**             | where the working copies of the projects live (one per project, `repos/<id>`)                    |
| `pack`           | `pack`                     | the pack binary a release's init runs (`POST /api/repo/{id}/init`)                               |
| `secrets`        | —                          | where the git credentials are read from — the same store the operator reads; only `openbao`      |
| `git.committer`  | `pack-ui <pack-ui@localhost>` | the committer of every commit pack-ui makes; `"Name <address>"`                               |
| `projects`       | —                          | the repositories pack-ui edits — a mirror of the operator's projects; a project is edited only when both know it |

## A project

```yaml
- id: dev            # the operator's project of the same id
  repo: https://git.example.org/platform/dev.git
  branch: main       # the main branch (default main)
  flow: direct       # direct — push to the main branch; review — push to a change request's branch
  credentials: git/dev   # the reference in the secrets store; empty — the remote asks nothing
```

The `flow` decides where a write lands ([editing.md](editing.md)): `direct` commits onto the main branch;
`review` refuses a write that does not name a change request's branch. A duplicate `id`, an unknown `flow`,
a project without `id` or `repo` — each is a fatal error at load.

## What load refuses

- no `operator`, no `repos`;
- `auth` not one of the three; `local` without users or with a user missing `name` or `password_hash`;
  `oidc` missing `issuer`, `client_id`, `client_secret` or `redirect_url`;
- `auth` on without `operator_token` — pack-ui could not prove itself to the operator;
- a `git.committer` that is not `"Name <address>"`;
- a `secrets.provider` that is not `openbao`;
- a project twice, or one without `id`/`repo`, or a `flow` that is neither `direct` nor `review`.

## A password hash for `users:`

```sh
go run golang.org/x/crypto/cmd/… # any bcrypt tool; e.g.:
htpasswd -bnBC 10 "" 'the password' | tr -d ':\n'
```
