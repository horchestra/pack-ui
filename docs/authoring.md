# Authoring a pack from pack-ui

The author's work (C6) runs in pack-ui, not through the operator — none of it is a deploy. Six endpoints:
scaffold a new pack from a spec, check a pack's source, the docs skeleton, the CEL sandbox, the authoring
reference and the example packs. Everything reads a throwaway checkout of a branch; the only write is
scaffold's commit.

## Scaffold: a pack from a spec

```
GET  /api/author/scaffold/schema          the JSON Schema a spec is constrained by
POST /api/repo/{id}/author/scaffold       {spec, lang, dir, message?, branch?}
```

A `Spec` (`internal/author/spec.go`) is a pack said short — what a model's output is constrained to by the
schema, or a person writes by hand:

```json
{
  "name": "redis", "version": "0.1.0", "image": "redis:7",
  "title": "Redis", "doc": "A Redis instance.",
  "fields": [
    {"name": "port", "type": "integer", "default": 6379, "title": "The port"},
    {"name": "password", "type": "secret", "title": "The password", "required": true}
  ],
  "root": { /* PackRoot: install/upgrade/remove, the files */ },
  "compute": {"/port": "…CEL…"},
  "rules": {"/port": [{"when": "…", "say": "…"}]},
  "suggest": {"/password": {…}},
  "diagnose": [{"class": "…", "when": "…", "cause": "…", "fix": {…}}],
  "actions": [{"name": "restart", "…": "…"}]
}
```

`Scaffold` writes the source in `lang` (`go` | `python` | `java`) under `dir` (`packs/<name>`): the pack's
struct with the defaults and the jsonschema tags, `Meta`, `PackRoot`, `docs/<lang>.yaml` of each language,
the `content/` files the root names, and `tests/basic.yaml` — a scenario with every required setting given
a value, so `author_check` runs on a new pack at once. The files are committed as the user and the operator
is synced. The spec is not kept in step with the source afterwards: once written, the source is what the
pack is.

## Check: `pack author check` without a terminal

```
POST /api/repo/{id}/author/check   {path, branch?}   — the pack's directory in the repository
```

1. the source's language is detected (the files it has);
2. the source's own generator runs (`sdk.Run` in Go, `packsdk.main` in Python, `PackSdk.main` in Java) —
   a compiler's `file:line` or the SDK's refusal comes back as `generate`;
3. the generated pack is checked (the bundle's problems, the docs gaps) and every problem is mapped back
   to the **source** — `postgres.go:36`, not `/conf/port` (`author.Locate`: a json tag in Go, a field in
   Python, a `json =` or a camel field in Java);
4. the scenarios of `tests/*.yaml` run on the memory target.

The answer is `{ok, lang, generate?, problems?, scenarios?}` — a problem carries `pack`, `path`, `message`,
`warning`, and `source` (`file:line`).

The generation wants the language's runtime. `docker/pack-author` is the sandbox image with all three
(Go, Python, a JDK); a checkout mounts at `/work`.

## Docs: the skeleton of a language

```
POST /api/repo/{id}/author/docs   {path, lang, branch?}
```

The `docs/<lang>.yaml` skeleton of the pack (`pack docs skeleton`) — every key of the schema with its title
and a place for the description, in the language asked.

## The CEL sandbox

```
POST /api/repo/{id}/author/cel   {path, key?, expr, values?, facts?, branch?}
```

The expression worked out as a formula of the key would read it (`render.Eval`): on the `values`, on the
`facts`, the target a stub. `key` empty — the pack's only key. A bad expression or one the values cannot
feed is `values_invalid` with the eval's error. This is where an author tries a `compute` formula or a
rule's `when` before writing it into the source.

## The reference and the examples

```
GET /api/author/ref?topic=&lang=       a topic of pack's authoring guide
GET /api/author/example?name=&lang=    an example pack's source
```

`ref` cuts a topic out of pack's `docs/authoring.md`, the code blocks of other languages left out. The
topics: `struct`, `root`, `cel`, `suggest`, `diagnose`, `action`, `files`, `packages`, `docs`, `holds`,
`generate`, `test`. `lang` defaults to `go`.

`example` is a ready pack to start from: `postgres`, `openbao`, `docker`, each in `go`, `python` and
`java` — the files as one `{name: content}` map.
