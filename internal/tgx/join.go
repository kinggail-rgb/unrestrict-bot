package tgx

import (
	"context"
	"time"

	"github.com/gotd/td/tg"
)

// JoinInvite joins a private chat by invite hash, then archives and mutes it.
// Already being a participant is not an error.
func (c *Client) JoinInvite(ctx context.Context, hash string) (string, error) {
	peer, err := c.Peers.ImportInvite(ctx, hash)
	if err != nil {
		return "", err
	}
	input := peer.InputPeer()

	if _, err := c.API.FoldersEditPeerFolders(ctx, []tg.InputFolderPeer{
		{Peer: input, FolderID: 1},
	}); err != nil {
		c.logWarn("archive chat after join", err)
	}

	muteUntil := int(time.Date(2050, 12, 12, 0, 0, 0, 0, time.UTC).Unix())
	if _, err := c.API.AccountUpdateNotifySettings(ctx, &tg.AccountUpdateNotifySettingsRequest{
		Peer: &tg.InputNotifyPeer{Peer: input},
		Settings: tg.InputPeerNotifySettings{
			MuteUntil: muteUntil,
		},
	}); err != nil {
		c.logWarn("mute chat after join", err)
	}

	return peer.VisibleName(), nil
}

func (c *Client) logWarn(msg string, err error) {
	if c.log != nil {
		c.log.Warn(msg, "err", err)
	}
}
