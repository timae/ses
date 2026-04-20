// share-server is the hosted side of `ses share`: a small HTTP service that
// stores gzipped session transcripts with an expiry and serves them back at
// unguessable URLs.
//
// Config comes from env vars so the same binary works locally and on deploio:
//
//	SHARE_BEARER          — required; the static bearer token the CLI sends
//	SHARE_PUBLIC_URL      — required; externally-visible base URL (e.g. https://share.example.com)
//	SHARE_DATA_DIR        — storage root for gzipped shares (default: ./data/shares)
//	SHARE_ADDR            — listen address (default: :8080)
//	SHARE_SWEEP_INTERVAL  — how often the expiry sweeper runs (default: 15m)
//	SHARE_MAX_BODY_BYTES  — upload cap in bytes (default: 10485760 = 10 MB)
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/timae/rel.ai/internal/shareserver"
)

func main() {
	logger := log.New(os.Stdout, "share-server ", log.LstdFlags|log.Lmsgprefix)

	bearer := mustEnv("SHARE_BEARER", logger)
	publicURL := mustEnv("SHARE_PUBLIC_URL", logger)
	dataDir := envOr("SHARE_DATA_DIR", "./data/shares")
	addr := envOr("SHARE_ADDR", ":8080")
	sweepInterval := envDuration("SHARE_SWEEP_INTERVAL", 15*time.Minute, logger)
	maxBody := envInt64("SHARE_MAX_BODY_BYTES", 10<<20, logger)

	store, err := shareserver.NewFSStore(dataDir)
	if err != nil {
		logger.Fatalf("init store: %v", err)
	}
	srv, err := shareserver.NewServer(shareserver.Config{
		BearerToken:  bearer,
		PublicURL:    publicURL,
		MaxBodyBytes: maxBody,
		Logger:       logger,
	}, store)
	if err != nil {
		logger.Fatalf("init server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go runSweeper(ctx, store, sweepInterval, logger)

	go func() {
		logger.Printf("listening on %s, public URL %s, data dir %s", addr, publicURL, dataDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown: %v", err)
	}
}

// runSweeper deletes expired shares on an interval. Lifecycle rules on the
// underlying storage should be the primary mechanism once we move off the
// filesystem; this is the belt-and-braces backup.
func runSweeper(ctx context.Context, store shareserver.Store, interval time.Duration, logger *log.Logger) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ids, err := store.ListExpired(ctx, time.Now())
			if err != nil {
				logger.Printf("sweep list: %v", err)
				continue
			}
			for _, id := range ids {
				if err := store.Delete(ctx, id); err != nil {
					logger.Printf("sweep delete %s: %v", id, err)
				}
			}
			if len(ids) > 0 {
				logger.Printf("swept %d expired share(s)", len(ids))
			}
		}
	}
}

func mustEnv(key string, logger *log.Logger) string {
	v := os.Getenv(key)
	if v == "" {
		logger.Fatalf("missing required env var %s", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration, logger *log.Logger) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		logger.Fatalf("invalid %s=%q: %v", key, v, err)
	}
	return d
}

func envInt64(key string, def int64, logger *log.Logger) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		logger.Fatalf("invalid %s=%q: %v", key, v, err)
	}
	return n
}
