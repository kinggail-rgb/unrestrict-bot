// Package pipeline turns a parsed link into re-sent media: resolve the source,
// post a status message, serve from cache if possible, otherwise download and
// re-upload into the destination chat.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kamuridesu/unrestrict-bot/internal/cache"
	"github.com/kamuridesu/unrestrict-bot/internal/health"
	"github.com/kamuridesu/unrestrict-bot/internal/link"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"

	"github.com/gotd/td/tg"
)

// Pipeline holds shared dependencies. User is always used to read and download.
type Pipeline struct {
	User    *tgx.Client
	Cache   *cache.Cache
	TempDir string
	Log     *slog.Logger

	// Healthy, if set, is called whenever the pipeline makes progress.
	Healthy func()
}

// Target is where a result is delivered. Send owns Peer (the user client for the
// control chat, the bot client for DMs).
type Target struct {
	Send     *tgx.Client
	Peer     tg.InputPeerClass
	PeerID   int64
	ReplyTo  int
	UseCache bool

	// OnPost, if set, receives the id of every message the pipeline creates in
	// Peer, so a watcher can ignore its own output.
	OnPost func(id int)
}

func (t Target) posted(id int) {
	if t.OnPost != nil && id > 0 {
		t.OnPost(id)
	}
}

func (p *Pipeline) ok() {
	if p.Healthy != nil {
		p.Healthy()
	}
}

// HandleInvite joins a private chat and reports the outcome into the target.
func (p *Pipeline) HandleInvite(ctx context.Context, hash string, t Target) error {
	status := p.sendStatus(ctx, t, "Joining group…")
	name, err := p.User.JoinInvite(ctx, hash)
	p.ok()
	if err != nil {
		p.editOrSend(ctx, t, status, fmt.Sprintf("Could not join: %v", err))
		return err
	}
	health.Joins.Add(1)
	msg := "Group joined!"
	if name != "" {
		msg = fmt.Sprintf("Joined %q.", name)
	}
	p.editOrSend(ctx, t, status, msg)
	return nil
}

// HandleMessage resolves a message link and delivers its media.
func (p *Pipeline) HandleMessage(ctx context.Context, l link.Link, t Target) error {
	src, err := p.User.FetchMessage(ctx, l)
	if err != nil {
		return fmt.Errorf("fetch message: %w", err)
	}
	if src == nil {
		p.sendStatus(ctx, t,
			"Could not get the message. It was deleted, or the account cannot see that chat — send me the invite link first.")
		return nil
	}
	srcChatID := tgx.PeerID(src.Peer, p.User.SelfID)

	status := p.sendStatus(ctx, t, "Checking message…")

	if t.UseCache && p.Cache != nil {
		if e, ok := p.Cache.GetMedia(srcChatID, l.MessageID); ok && e.DestChatID == t.PeerID && e.DestMsgID > 0 {
			if fwID, ferr := t.Send.Forward(ctx, t.Peer, e.DestMsgID, t.Peer); ferr == nil {
				t.posted(fwID)
				health.CacheHits.Add(1)
				health.Processed.Add(1)
				p.deleteStatus(ctx, t, status)
				p.ok()
				p.Log.Info("served from cache", "src_chat", srcChatID, "src_msg", l.MessageID)
				return nil
			}
			_ = p.Cache.DeleteMedia(srcChatID, l.MessageID)
		}
	}

	if src.Msg.Media == nil {
		text := src.Msg.Message
		if text == "" {
			text = "That message has no media."
		}
		p.editOrSend(ctx, t, status, text)
		p.ok()
		return nil
	}

	p.edit(ctx, t, status, "Downloading media…")
	// progress is invoked (already throttled) from concurrent download workers.
	progress := func(done, total int64) {
		if total <= 0 {
			return
		}
		p.edit(ctx, t, status, fmt.Sprintf("Downloading… %d%%", done*100/total))
	}

	res, err := p.User.Download(ctx, src.Msg, p.TempDir, progress)
	if err != nil {
		health.Failures.Add(1)
		p.editOrSend(ctx, t, status, fmt.Sprintf("Could not download media: %v", err))
		return fmt.Errorf("download: %w", err)
	}
	if res.Path == "" || res.Kind == tgx.KindNone {
		text := src.Msg.Message
		if text == "" {
			text = "That message has no downloadable media."
		}
		p.editOrSend(ctx, t, status, text)
		return nil
	}
	defer func() { _ = os.Remove(res.Path) }()
	health.DownloadBytes.Add(res.Size)

	p.edit(ctx, t, status, "Sending media…")

	var newID int
	var sendErr error
	for attempt := 0; attempt < 5; attempt++ {
		newID, sendErr = t.Send.SendMediaFile(ctx, t.Peer, t.ReplyTo, res, src.Msg.Message)
		if sendErr == nil {
			break
		}
		p.Log.Warn("send media failed, retrying", "attempt", attempt+1, "err", sendErr)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if sendErr != nil {
		health.Failures.Add(1)
		p.editOrSend(ctx, t, status, "Could not send the media.")
		return fmt.Errorf("send media: %w", sendErr)
	}
	t.posted(newID)

	if t.UseCache && p.Cache != nil && newID > 0 {
		_ = p.Cache.PutMedia(srcChatID, l.MessageID, cache.Entry{DestChatID: t.PeerID, DestMsgID: newID})
	}

	p.deleteStatus(ctx, t, status)
	health.Processed.Add(1)
	p.ok()
	p.Log.Info("delivered media",
		"src_chat", srcChatID, "src_msg", l.MessageID, "bytes", res.Size, "kind", res.Kind)
	return nil
}

func (p *Pipeline) sendStatus(ctx context.Context, t Target, text string) int {
	id, err := t.Send.SendText(ctx, t.Peer, t.ReplyTo, text)
	if err != nil {
		p.Log.Debug("send status failed", "err", err)
		return 0
	}
	t.posted(id)
	return id
}

func (p *Pipeline) edit(ctx context.Context, t Target, id int, text string) {
	if id <= 0 {
		return
	}
	if err := t.Send.EditText(ctx, t.Peer, id, text); err != nil {
		p.Log.Debug("edit status failed", "err", err)
	}
}

func (p *Pipeline) editOrSend(ctx context.Context, t Target, id int, text string) {
	if id > 0 {
		if err := t.Send.EditText(ctx, t.Peer, id, text); err == nil {
			return
		}
	}
	p.sendStatus(ctx, t, text)
}

func (p *Pipeline) deleteStatus(ctx context.Context, t Target, id int) {
	if id <= 0 {
		return
	}
	if err := t.Send.DeleteMessage(ctx, t.Peer, id); err != nil {
		p.Log.Debug("delete status failed", "err", err)
	}
}
