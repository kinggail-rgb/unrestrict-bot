package app

import (
	"context"
	"fmt"
	"os"

	"github.com/kamuridesu/unrestrict-bot/internal/cache"
	"github.com/kamuridesu/unrestrict-bot/internal/config"
	"github.com/kamuridesu/unrestrict-bot/internal/session"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"
)

// GenSession runs an interactive login and saves the session to the database.
func GenSession(ctx context.Context) error {
	cfg, err := config.Load(config.ModeGenSession)
	if err != nil {
		return err
	}

	c, err := cache.Open(cfg.CachePath)
	if err != nil {
		return err
	}
	defer c.Close()

	store, err := session.NewStore(c.DB(), "user")
	if err != nil {
		return err
	}

	if err := tgx.Login(ctx, cfg.APIID, cfg.APIHash, store, interactiveFlow()); err != nil {
		return err
	}

	raw, _ := store.LoadSession(ctx)
	fmt.Fprintf(os.Stderr, "\nLogged in. Session saved to %s.\n", cfg.CachePath)
	fmt.Fprint(os.Stderr, "Portable SESSION_STRING (optional, for other hosts):\n\n")
	fmt.Println(session.Encode(raw))
	return nil
}
