// Package bot implements the optional "DM the bot a link" mode.
package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/kamuridesu/unrestrict-bot/internal/link"
	"github.com/kamuridesu/unrestrict-bot/internal/pipeline"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"

	"github.com/gotd/td/tg"
)

const helpText = `Send me a link to a public group and I'll forward the message to you.
To save content from a private group, first send me its invite link so my account can see the messages too.
Source: https://github.com/kamuridesu/unrestrict-bot`

// Bot handles updates for the bot account.
type Bot struct {
	Bot      *tgx.Client // the bot client (owns the destination DMs)
	Pipeline *pipeline.Pipeline
	Log      *slog.Logger
}

// Register installs update handlers on the bot client's dispatcher.
func (b *Bot) Register(ctx context.Context) error {
	b.Bot.Dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok || msg.Out || msg.Message == "" || msg.Media != nil {
			return nil
		}
		peerUser, ok := msg.PeerID.(*tg.PeerUser)
		if !ok {
			return nil
		}
		var inputPeer tg.InputPeerClass = &tg.InputPeerUser{UserID: peerUser.UserID}
		if usr, ok := e.Users[peerUser.UserID]; ok {
			inputPeer = &tg.InputPeerUser{UserID: usr.ID, AccessHash: usr.AccessHash}
		}

		text := strings.TrimSpace(msg.Message)
		if text == "/start" || text == "/help" || strings.HasPrefix(text, "/start ") {
			_, _ = b.Bot.SendText(ctx, inputPeer, msg.ID, helpText)
			return nil
		}

		l := link.Find(text)
		if l.Kind == link.KindUnknown {
			_, _ = b.Bot.SendText(ctx, inputPeer, msg.ID, "Send me a t.me message link or a private group invite link.")
			return nil
		}

		t := pipeline.Target{
			Send:     b.Bot,
			Peer:     inputPeer,
			PeerID:   peerUser.UserID,
			ReplyTo:  msg.ID,
			UseCache: false,
		}
		go func() {
			var err error
			switch l.Kind {
			case link.KindInvite:
				err = b.Pipeline.HandleInvite(ctx, l.InviteHash, t)
			case link.KindMessage:
				err = b.Pipeline.HandleMessage(ctx, l, t)
			}
			if err != nil {
				b.Log.Error("bot pipeline failed", "err", err, "link", l.Raw)
			}
		}()
		return nil
	})
	return nil
}

// Run blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
