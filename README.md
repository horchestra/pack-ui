# pack-ui

The web UI of [pack](https://github.com/horchestra/pack): one binary — the SPA (embedded) and its backend,
which edits the projects' repositories as the user (go-git) and proxies the operator's API under the
session's identity. A second mode, `pack-ui mcp`, is an MCP server a model works the values and the packs
with.

**The documentation is [docs/](docs/README.md).**

## Build and run

```sh
cd web && npm ci && npm run build   # the SPA into web/dist, embedded by the next step
go build -o bin/pack-ui ./cmd/pack-ui
./bin/pack-ui --config pack-ui.yaml
```

## Test

```sh
go test -race ./...                  # the backend
(cd web && npm test)                 # the SPA
(cd e2e && npx playwright test)      # end-to-end, needs PACK_UI_URL of a live system
```

CI (`.github/workflows/ci.yml`): the web build and vitest, `go mod tidy -diff`, `gofmt`, `go vet`,
`go test -race ./...`.
