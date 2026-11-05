// pack-ui is the web UI of pack: the SPA and its backend — the operator's API client (К4) and the git
// client of the users' edits (К5).
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	opgit "github.com/horchestra/pack-operator/git"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/mcpserver"
	"github.com/horchestra/pack-ui/internal/operator"
	"github.com/horchestra/pack-ui/internal/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		mcpMain(os.Args[2:])
		return
	}
	path := flag.String("config", "pack-ui.yaml", "the configuration file")
	flag.Parse()
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	cfg, err := config.Load(*path)
	if err != nil {
		log.Fatal().Err(err).Msg("the configuration")
	}
	var store opgit.Store
	if cfg.Secrets.Provider == "openbao" {
		store = opgit.OpenBao{Addr: cfg.Secrets.Addr, Mount: cfg.Secrets.Mount, TokenEnv: cfg.Secrets.TokenEnv}
	}
	repos := edit.New(cfg.Repos, store, cfg.Git.Committer)
	defer repos.Close()
	op := &operator.Client{Base: cfg.Operator, User: cfg.User, Token: cfg.OperatorToken}
	srv := &http.Server{Addr: cfg.Listen, Handler: server.New(cfg, op, repos).Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info().Str("listen", cfg.Listen).Str("operator", cfg.Operator).Msg("pack-ui")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("the server")
	}
}

// mcpMain is `pack-ui mcp`: the MCP server a model works the values and the packs with, on stdin and
// stdout, against a running pack-ui (-—base, its user --user). What is a person's stays a person's: no
// tool approves a deploy or merges a change.
func mcpMain(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	base := fs.String("base", envOr("PACK_UI_URL", "http://127.0.0.1:8090"), "pack-ui's URL (PACK_UI_URL)")
	user := fs.String("user", envOr("PACK_UI_USER", ""), "the X-Pack-User header, empty — none (PACK_UI_USER)")
	token := fs.String("token", envOr("PACK_UI_TOKEN", ""), "a CI token of pack-ui (PACK_UI_TOKEN; auth local|oidc wants one)")
	_ = fs.Parse(args)
	srv := mcpserver.New(&mcpserver.Client{Base: *base, User: *user, Token: *token}, "0.1.0")
	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal().Err(err).Msg("the MCP server")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); len(v) > 0 {
		return v
	}
	return fallback
}
