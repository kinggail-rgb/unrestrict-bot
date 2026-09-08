// Package tgx wraps gotd/td: a gaps-aware update dispatcher, a persistent peer
// manager, flood-wait/rate-limit middleware, and fetch/download/send helpers.
package tgx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// Client is a running gotd client plus helpers.
type Client struct {
	API        *tg.Client
	Sender     *message.Sender
	Peers      *peers.Manager
	Dispatcher tg.UpdateDispatcher
	Downloader *downloader.Downloader
	Uploader   *uploader.Uploader

	tg     *telegram.Client
	gaps   *updates.Manager
	log    *slog.Logger
	auth   func(ctx context.Context, c *telegram.Client) error
	SelfID int64
	IsBot  bool
}

// Options configures a Client.
type Options struct {
	AppID   int
	AppHash string

	SessionStorage telegram.SessionStorage
	PeerDB         *bolt.DB
	Logger         *slog.Logger

	// Auth is invoked inside Run to ensure the client is authorized.
	Auth func(ctx context.Context, c *telegram.Client) error
}

// New builds an unstarted Client.
func New(opt Options) (*Client, error) {
	var peerStorage peers.Storage
	if opt.PeerDB != nil {
		bs, err := newBoltPeerStorage(opt.PeerDB)
		if err != nil {
			return nil, fmt.Errorf("peer storage: %w", err)
		}
		peerStorage = bs
	}

	dispatcher := tg.NewUpdateDispatcher()

	// Assigned below; the closures break the client/peers/gaps construction cycle.
	var (
		h    telegram.UpdateHandler
		gaps *updates.Manager
	)
	waiter := floodwait.NewSimpleWaiter()

	client := telegram.NewClient(opt.AppID, opt.AppHash, telegram.Options{
		SessionStorage: opt.SessionStorage,
		UpdateHandler: telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			return h.Handle(ctx, u)
		}),
		Middlewares: []telegram.Middleware{
			waiter,
			ratelimit.New(rate.Every(100*time.Millisecond), 5),
			updhook.UpdateHook(func(ctx context.Context, u tg.UpdatesClass) error {
				return gaps.Handle(ctx, u)
			}),
		},
	})

	pm := peers.Options{Storage: peerStorage}.Build(client.API())
	gaps = updates.New(updates.Config{
		Handler:      dispatcher,
		AccessHasher: pm,
	})
	h = pm.UpdateHook(gaps)

	up := uploader.NewUploader(client.API()).WithThreads(4)
	sender := message.NewSender(client.API()).WithUploader(up)

	return &Client{
		API:        client.API(),
		Sender:     sender,
		Peers:      pm,
		Dispatcher: dispatcher,
		Downloader: downloader.NewDownloader(),
		Uploader:   up,
		tg:         client,
		gaps:       gaps,
		log:        opt.Logger,
		auth:       opt.Auth,
	}, nil
}

// Run connects, authorizes, starts update handling and calls f, blocking until
// ctx is cancelled or an error occurs.
func (c *Client) Run(ctx context.Context, f func(ctx context.Context) error) error {
	return c.tg.Run(ctx, func(ctx context.Context) error {
		if c.auth != nil {
			if err := c.auth(ctx, c.tg); err != nil {
				return err
			}
		}
		status, err := c.tg.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		if !status.Authorized {
			return fmt.Errorf("not authorized: run `unrestrict-bot gen-session` interactively, or set SESSION_STRING")
		}

		if err := c.Peers.Init(ctx); err != nil {
			return fmt.Errorf("peers init: %w", err)
		}
		self, err := c.Peers.Self(ctx)
		if err != nil {
			return fmt.Errorf("resolve self: %w", err)
		}
		c.SelfID = self.ID()
		_, c.IsBot = self.ToBot()

		grp, gctx := errgroup.WithContext(ctx)
		grp.Go(func() error {
			return c.gaps.Run(gctx, c.API, self.ID(), updates.AuthOptions{IsBot: c.IsBot})
		})
		grp.Go(func() error { return f(gctx) })
		return grp.Wait()
	})
}

// UserAuth relies on an existing session; it performs no login.
func UserAuth() func(ctx context.Context, c *telegram.Client) error {
	return func(ctx context.Context, c *telegram.Client) error { return nil }
}

// BotAuth performs a bot-token login if the client is not already authorized.
func BotAuth(token string) func(ctx context.Context, c *telegram.Client) error {
	return func(ctx context.Context, c *telegram.Client) error {
		status, err := c.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized {
			return nil
		}
		if _, err := c.Auth().Bot(ctx, token); err != nil {
			return fmt.Errorf("bot login: %w", err)
		}
		return nil
	}
}

// Login authenticates against storage using flow (if not already authorized),
// persisting the session there. It is a no-op when storage already holds a
// valid session.
func Login(ctx context.Context, appID int, appHash string, storage telegram.SessionStorage, flow auth.Flow) error {
	client := telegram.NewClient(appID, appHash, telegram.Options{SessionStorage: storage})
	return client.Run(ctx, func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return err
		}
		_, err := client.Self(ctx)
		return err
	})
}
