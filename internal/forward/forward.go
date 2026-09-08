// Package forward implements the bulk media-forward mode: copy every photo/video
// from FORWARD_FROM into FORWARD_TO, oldest first, resuming from saved progress.
package forward

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/kamuridesu/unrestrict-bot/internal/cache"
	"github.com/kamuridesu/unrestrict-bot/internal/tgx"

	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const batchSize = 100

// Runner performs the bulk forward.
type Runner struct {
	User  *tgx.Client
	Cache *cache.Cache
	Log   *slog.Logger

	From string
	To   string
}

// Run collects media message ids newer than the saved progress and forwards them
// in batches.
func (r *Runner) Run(ctx context.Context) error {
	fromPeer, _, err := r.User.ResolvePeer(ctx, r.From)
	if err != nil {
		return fmt.Errorf("resolve FORWARD_FROM: %w", err)
	}
	toPeer, _, err := r.User.ResolvePeer(ctx, r.To)
	if err != nil {
		return fmt.Errorf("resolve FORWARD_TO: %w", err)
	}

	progressKey := r.From + ":" + r.To
	last := r.Cache.ForwardProgress(progressKey)
	r.Log.Info("bulk forward starting", "from", r.From, "to", r.To, "resume_after", last)

	var ids []int
	iter := query.NewQuery(r.User.API).Messages().GetHistory(fromPeer).BatchSize(batchSize).Iter()
	for iter.Next(ctx) {
		m := iter.Value().Msg
		msg, ok := m.(*tg.Message)
		if !ok || msg.ID <= last {
			continue
		}
		if !hasMedia(msg) {
			continue
		}
		ids = append(ids, msg.ID)
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("iterate history: %w", err)
	}
	sort.Ints(ids)
	r.Log.Info("collected messages to forward", "count", len(ids))

	for i := 0; i < len(ids); i += 500 {
		end := min(i+500, len(ids))
		batch := ids[i:end]
		if err := r.forwardBatch(ctx, fromPeer, toPeer, batch); err != nil {
			return err
		}
		if err := r.Cache.SetForwardProgress(progressKey, batch[len(batch)-1]); err != nil {
			r.Log.Warn("save progress failed", "err", err)
		}
		r.Log.Info("forwarded batch", "count", len(batch), "through_id", batch[len(batch)-1])
	}
	r.Log.Info("bulk forward done")
	return nil
}

func (r *Runner) forwardBatch(ctx context.Context, from, to tg.InputPeerClass, ids []int) error {
	randomIDs := make([]int64, len(ids))
	for i := range randomIDs {
		var b [8]byte
		_, _ = rand.Read(b[:])
		randomIDs[i] = int64(binary.LittleEndian.Uint64(b[:]))
	}

	req := &tg.MessagesForwardMessagesRequest{
		FromPeer: from,
		ToPeer:   to,
		ID:       ids,
		RandomID: randomIDs,
	}

	for attempt := 0; attempt < 5; attempt++ {
		_, err := r.User.API.MessagesForwardMessages(ctx, req)
		if err == nil {
			return nil
		}
		if wait, ok := tgerr.AsFloodWait(err); ok {
			r.Log.Warn("flood wait", "duration", wait.String())
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait + time.Second):
			}
			continue
		}
		return fmt.Errorf("forward messages: %w", err)
	}
	return fmt.Errorf("forward messages: too many flood waits")
}

func hasMedia(msg *tg.Message) bool {
	return tgx.MediaKindOf(msg) != tgx.KindNone
}
