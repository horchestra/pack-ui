# pack-ui mcp: the model's tools

`pack-ui mcp` is an MCP server on stdin and stdout (`internal/mcpserver`). It gives a model the tools to
work a release's values and a pack's source with — every tool a plain call of pack-ui's own API, so every
rule of the API applies to a model exactly as to a person: the write is a commit, the conflict is a 409,
the auth is the configuration's.

```sh
pack-ui mcp --base http://127.0.0.1:8090 --user alice --token s3cr3t
```

| flag      | environment   | default                 | what it is                                                        |
|-----------|---------------|-------------------------|-------------------------------------------------------------------|
| `--base`  | `PACK_UI_URL` | `http://127.0.0.1:8090` | the pack-ui the tools call                                        |
| `--user`  | `PACK_UI_USER`| —                       | the `X-Pack-User` header; empty — none (auth `none` then decides) |
| `--token` | `PACK_UI_TOKEN`| —                      | a CI token of the configuration's `tokens:`; `auth local`/`oidc` wants one — a session is a person's, a model carries a token |

## The tools

### The values

- **questions** `{project, path, values, lang?}` → `values/check`. What the values still ask: the settings
  required and missing, the suggestions, the problems — every one where the schema points.
- **answer** `{project, path, values, message, lang?}` → `values/write`. The values written whole, rendered
  as `pack values write` renders them, committed as the user, the operator synced. The commit is one a
  person reviews — the tool's description tells the model to say what it wrote and why, and the `message`
  is where.

### The diagnosis

- **diagnose** `{project, path, saga?, key?, lang?}` → `diagnose`. The class, the cause, the evidence of
  the journal and the logs, the ways out. A patch is offered and the **person** confirms it (the values'
  endpoints write it); a command the person runs as a request of the operator.

### The authoring

- **author_check** `{project, path, lang?}` — generate the pack's source and check it: the generate
  problems, the checks with where in the source, the scenarios' runs.
- **author_docs** `{project, path, lang?}` — the `docs/<lang>.yaml` skeleton.
- **author_cel** `{project, path, key?, expr, values?, facts?}` — a CEL expression worked out as a formula
  of the key would read it.
- **author_scaffold** `{project, spec, lang, dir, message?}` — a new pack's source from a spec, written
  into the repository, committed. The spec's schema is `GET /api/author/scaffold/schema`.
- **author_ref** `{lang?}`, **author_example** `{name, lang?}` — the authoring guide's topics, the ready
  example packs.

([values.md](values.md), [diagnosis.md](diagnosis.md), [authoring.md](authoring.md) say what each endpoint
does; the tools inherit it all, the errors come back as the Failure's `code: message`.)

## What no tool does

What is a person's stays a person's:

- **no tool approves a deploy** — a request is signed by a person;
- **no tool merges a change request**;
- **no tool reads a secret's value** — a password is a reference, a name and a key.
