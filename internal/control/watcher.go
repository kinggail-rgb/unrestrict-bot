// Package control watches a chat (Saved Messages by default) for Telegram links
// and unrestricts them back into that same chat.
package control

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/kamuridesu/unrestrict-bot/internal/link"
	"github.com/kamuridesu/unrestrict-bot/internal/pipeline"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"

	"github.com/gotd/td/tg"
)

// Watcher wires the user client's update dispatcher to the pipeline.
type Watcher struct {
	User     *tgx.Client
	Pipeline *pipeline.Pipeline
	Log      *slog.Logger

	ChatSpec string // CONTROL_CHAT

	ready  atomic.Bool
	peer   tg.InputPeerClass
	peerID int64

	mu      sync.Mutex
	handled map[int]struct{} // message ids we created or already processed
}

// Attach installs update handlers on the dispatcher. Call before Client.Run.
// Handlers no-op until Resolve has run.
func (w *Watcher) Attach() {
	w.handled = make(map[int]struct{})
	w.User.Dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		w.dispatch(ctx, u.Message)
		return nil
	})
	w.User.Dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		w.dispatch(ctx, u.Message)
		return nil
	})
}

// Resolve looks up the control chat. Call from inside Client.Run (after auth).
func (w *Watcher) Resolve(ctx context.Context) error {
	peer, id, err := w.User.ResolvePeer(ctx, w.ChatSpec)
	if err != nil {
		return err
	}
	w.peer, w.peerID = peer, id
	w.ready.Store(true)
	w.Log.Info("watching control chat", "chat_id", id, "spec", w.ChatSpec)
	return nil
}

// Run blocks until ctx is cancelled (updates are delivered via the dispatcher).
func (w *Watcher) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (w *Watcher) markHandled(id int) {
	w.mu.Lock()
	w.handled[id] = struct{}{}
	w.mu.Unlock()
}

func (w *Watcher) seen(id int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.handled[id]
	if !ok {
		w.handled[id] = struct{}{}
	}
	return ok
}

func (w *Watcher) dispatch(ctx context.Context, m tg.MessageClass) {
	if !w.ready.Load() {
		return
	}
	msg, ok := m.(*tg.Message)
	if !ok || msg.Message == "" {
		return
	}
	// Triggers are links in a plain text message. Skip anything with media so we
	// never react to our own re-posted media (whose caption echoes the source).
	if msg.Media != nil {
		return
	}
	if !peerEqual(msg.PeerID, w.peer, w.peerID) {
		return
	}
	if w.seen(msg.ID) {
		return
	}

	l := link.Find(msg.Message, entityURLs(msg)...)
	if l.Kind == link.KindUnknown {
		return
	}

	t := pipeline.Target{
		Send:     w.User,
		Peer:     w.peer,
		PeerID:   w.peerID,
		ReplyTo:  msg.ID,
		UseCache: true,
		OnPost:   w.markHandled,
	}

	go func() {
		var err error
		switch l.Kind {
		case link.KindInvite:
			err = w.Pipeline.HandleInvite(ctx, l.InviteHash, t)
		case link.KindMessage:
			err = w.Pipeline.HandleMessage(ctx, l, t)
		}
		if err != nil {
			w.Log.Error("pipeline failed", "err", err, "link", l.Raw)
		}
	}()
}

func entityURLs(msg *tg.Message) []string {
	var out []string
	runes := []rune(msg.Message)
	for _, ent := range msg.Entities {
		switch e := ent.(type) {
		case *tg.MessageEntityTextURL:
			if e.URL != "" {
				out = append(out, e.URL)
			}
		case *tg.MessageEntityURL:
			if e.Offset >= 0 && e.Offset+e.Length <= len(runes) {
				out = append(out, string(runes[e.Offset:e.Offset+e.Length]))
			}
		}
	}
	return out
}

func peerEqual(p tg.PeerClass, want tg.InputPeerClass, wantID int64) bool {
	switch pv := p.(type) {
	case *tg.PeerUser:
		if _, ok := want.(*tg.InputPeerSelf); ok {
			return pv.UserID == wantID
		}
		_, ok := want.(*tg.InputPeerUser)
		return ok && pv.UserID == wantID
	case *tg.PeerChat:
		return pv.ChatID == wantID
	case *tg.PeerChannel:
		return pv.ChannelID == wantID
	default:
		return false
	}
}
