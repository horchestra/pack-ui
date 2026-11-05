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

	"github.com/horchestra/pack-ui/internal/config"
	"github.com/horchestra/pack-ui/internal/edit"
	"github.com/horchestra/pack-ui/internal/operator"
	"github.com/horchestra/pack-ui/internal/server"
)

func main() {
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
	op := &operator.Client{Base: cfg.Operator, User: cfg.User}
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
