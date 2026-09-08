package tgx

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gotd/td/constant"
	"github.com/gotd/td/tg"
)

// ResolvePeer turns a control-chat specification into an input peer and its
// numeric id. Accepted forms: "me" (Saved Messages), "@username", "username",
// or a numeric (marked) chat id such as -1001234567890.
func (c *Client) ResolvePeer(ctx context.Context, spec string) (tg.InputPeerClass, int64, error) {
	spec = strings.TrimSpace(spec)
	switch strings.ToLower(spec) {
	case "", "me", "self", "saved", "saved messages":
		return &tg.InputPeerSelf{}, c.SelfID, nil
	}

	if n, err := strconv.ParseInt(spec, 10, 64); err == nil {
		p, rerr := c.Peers.ResolveTDLibID(ctx, constant.TDLibPeerID(n))
		if rerr != nil {
			return nil, 0, fmt.Errorf("resolve chat id %d: %w", n, rerr)
		}
		return p.InputPeer(), p.ID(), nil
	}

	p, err := c.Peers.Resolve(ctx, strings.TrimPrefix(spec, "@"))
	if err != nil {
		return nil, 0, fmt.Errorf("resolve %q: %w", spec, err)
	}
	return p.InputPeer(), p.ID(), nil
}

// PeerID extracts the numeric id from an input peer. InputPeerSelf resolves to
// selfID.
func PeerID(p tg.InputPeerClass, selfID int64) int64 {
	switch v := p.(type) {
	case *tg.InputPeerSelf:
		return selfID
	case *tg.InputPeerUser:
		return v.UserID
	case *tg.InputPeerChat:
		return v.ChatID
	case *tg.InputPeerChannel:
		return v.ChannelID
	case *tg.InputPeerUserFromMessage:
		return v.UserID
	case *tg.InputPeerChannelFromMessage:
		return v.ChannelID
	default:
		return 0
	}
}

// ResolveUserPeer returns an input peer for a user id known to the peer cache.
func (c *Client) ResolveUserPeer(ctx context.Context, userID int64) (tg.InputPeerClass, error) {
	u, err := c.Peers.ResolveUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return u.InputPeer(), nil
}
