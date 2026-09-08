// Package app wires the subcommands together.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kamuridesu/unrestrict-bot/internal/bot"
	"github.com/kamuridesu/unrestrict-bot/internal/cache"
	"github.com/kamuridesu/unrestrict-bot/internal/config"
	"github.com/kamuridesu/unrestrict-bot/internal/control"
	"github.com/kamuridesu/unrestrict-bot/internal/health"
	"github.com/kamuridesu/unrestrict-bot/internal/pipeline"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"

	"github.com/gotd/td/telegram"
	"golang.org/x/sync/errgroup"
)

const healthStale = 5 * time.Minute

// Run starts the service: the control-chat watcher, the optional bot, and the
// health server.
func Run(ctx context.Context) error {
	cfg, err := config.Load(config.ModeRun)
	if err != nil {
		return err
	}
	log := cfg.Logger()
	slog.SetDefault(log)

	c, err := cache.Open(cfg.CachePath)
	if err != nil {
		return err
	}
	defer c.Close()

	user, err := newUserClient(ctx, cfg, c, log.With("component", "user"))
	if err != nil {
		return err
	}

	checker := health.New(cfg.HealthFile, healthStale, log.With("component", "health"))

	pl := &pipeline.Pipeline{
		User:    user,
		Cache:   c,
		TempDir: cfg.TempDir,
		Log:     log.With("component", "pipeline"),
		Healthy: checker.OK,
	}

	watcher := &control.Watcher{
		User:     user,
		Pipeline: pl,
		Log:      log.With("component", "control"),
		ChatSpec: cfg.ControlChat,
	}
	watcher.Attach()

	grp, ctx := errgroup.WithContext(ctx)

	grp.Go(func() error { return checker.Serve(ctx, cfg.HealthAddr) })

	grp.Go(func() error {
		return user.Run(ctx, func(ctx context.Context) error {
			if err := watcher.Resolve(ctx); err != nil {
				return err
			}
			log.Info("user client ready", "self_id", user.SelfID)
			// Heartbeat: a connected, idle client is still healthy.
			go func() {
				t := time.NewTicker(time.Minute)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						checker.OK()
					}
				}
			}()
			return watcher.Run(ctx)
		})
	})

	if cfg.BotEnabled() {
		botClient, err := tgx.New(tgx.Options{
			AppID:   cfg.APIID,
			AppHash: cfg.APIHash,
			// The bot logs in fresh each start; keep its session in memory.
			SessionStorage: &memSession{},
			Logger:         log.With("component", "bot"),
			Auth:           tgx.BotAuth(cfg.BotToken),
		})
		if err != nil {
			return err
		}
		b := &bot.Bot{Bot: botClient, Pipeline: pl, Log: log.With("component", "bot")}
		if err := b.Register(ctx); err != nil {
			return err
		}
		grp.Go(func() error {
			return botClient.Run(ctx, func(ctx context.Context) error {
				log.Info("bot client ready", "self_id", botClient.SelfID)
				return b.Run(ctx)
			})
		})
	}

	if err := grp.Wait(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// memSession is a throwaway in-memory session store for the bot client.
type memSession struct {
	data []byte
}

func (m *memSession) LoadSession(context.Context) ([]byte, error) { return m.data, nil }
func (m *memSession) StoreSession(_ context.Context, d []byte) error {
	m.data = d
	return nil
}

var _ telegram.SessionStorage = (*memSession)(nil)
