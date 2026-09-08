package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kamuridesu/unrestrict-bot/internal/cache"
	"github.com/kamuridesu/unrestrict-bot/internal/config"
	"github.com/kamuridesu/unrestrict-bot/internal/session"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"
)

// newUserClient builds the user-account client. The session comes from the bbolt
// store; if it is empty it is seeded from SESSION_STRING, or an interactive login
// runs (when stdin is a terminal) and its session is saved back to the store.
func newUserClient(ctx context.Context, cfg *config.Config, c *cache.Cache, log *slog.Logger) (*tgx.Client, error) {
	store, err := session.NewStore(c.DB(), "user")
	if err != nil {
		return nil, err
	}
	if err := store.Seed(ctx, cfg.SessionString); err != nil {
		return nil, err
	}

	if !store.Has() {
		if !stdinIsTerminal() {
			return nil, fmt.Errorf("no saved session: run this interactively once (or `unrestrict-bot gen-session`), or set SESSION_STRING")
		}
		log.Info("no saved session, starting login")
		if err := tgx.Login(ctx, cfg.APIID, cfg.APIHash, store, interactiveFlow()); err != nil {
			return nil, fmt.Errorf("login: %w", err)
		}
		log.Info("logged in, session saved")
	}

	return tgx.New(tgx.Options{
		AppID:          cfg.APIID,
		AppHash:        cfg.APIHash,
		SessionStorage: store,
		PeerDB:         c.DB(),
		Logger:         log,
		Auth:           tgx.UserAuth(),
	})
}
