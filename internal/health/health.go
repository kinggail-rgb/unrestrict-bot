// Package health exposes an HTTP liveness/readiness endpoint plus expvar
// metrics, and keeps touching a file so the legacy docker-compose healthcheck
// keeps working.
package health

import (
	"context"
	"errors"
	"expvar"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// Metrics are process-wide counters published at /metrics.
var (
	Processed     = expvar.NewInt("unrestrict_processed_total")
	CacheHits     = expvar.NewInt("unrestrict_cache_hits_total")
	Failures      = expvar.NewInt("unrestrict_failures_total")
	DownloadBytes = expvar.NewInt("unrestrict_download_bytes_total")
	Joins         = expvar.NewInt("unrestrict_joins_total")
)

// Checker tracks the last time the service made progress and answers /healthz.
type Checker struct {
	lastOK   atomic.Int64 // unix nanos
	staleFor time.Duration
	file     string
	log      *slog.Logger
}

// New creates a Checker. Requests to /healthz fail once no progress has been
// reported for staleFor.
func New(file string, staleFor time.Duration, log *slog.Logger) *Checker {
	c := &Checker{staleFor: staleFor, file: file, log: log}
	c.OK()
	return c
}

// OK marks the service as healthy right now.
func (c *Checker) OK() {
	c.lastOK.Store(time.Now().UnixNano())
	c.touch()
}

func (c *Checker) healthy() bool {
	last := time.Unix(0, c.lastOK.Load())
	return time.Since(last) < c.staleFor
}

func (c *Checker) touch() {
	if c.file == "" {
		return
	}
	now := time.Now()
	if err := os.Chtimes(c.file, now, now); err != nil {
		if f, ferr := os.Create(c.file); ferr == nil {
			_ = f.Close()
		}
	}
}

// Serve runs the HTTP server until ctx is cancelled.
func (c *Checker) Serve(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if c.healthy() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("stale\n"))
	})
	mux.Handle("/metrics", expvar.Handler())

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	// Keep the healthcheck file fresh even when idle.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if c.healthy() {
					c.touch()
				}
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	c.log.Info("health server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
