package app

import (
	"context"
	"log/slog"

	"github.com/kamuridesu/unrestrict-bot/internal/cache"
	"github.com/kamuridesu/unrestrict-bot/internal/config"
	"github.com/kamuridesu/unrestrict-bot/internal/forward"
)

// Forward runs the bulk media-forward mode once and exits.
func Forward(ctx context.Context) error {
	cfg, err := config.Load(config.ModeForward)
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

	user, err := newUserClient(ctx, cfg, c, log)
	if err != nil {
		return err
	}

	runner := &forward.Runner{
		User:  user,
		Cache: c,
		Log:   log.With("component", "forward"),
		From:  cfg.ForwardFrom,
		To:    cfg.ForwardTo,
	}

	return user.Run(ctx, func(ctx context.Context) error {
		return runner.Run(ctx)
	})
}
