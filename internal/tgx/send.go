package tgx

import (
	"context"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

// SendText posts a plain-text message, optionally as a reply, returning its id.
func (c *Client) SendText(ctx context.Context, peer tg.InputPeerClass, replyTo int, text string) (int, error) {
	rb := c.Sender.To(peer)
	b := &rb.Builder
	if replyTo > 0 {
		b = rb.Reply(replyTo)
	}
	return unpack.MessageID(b.Text(ctx, text))
}

// EditText edits a message posted by this account.
func (c *Client) EditText(ctx context.Context, peer tg.InputPeerClass, id int, text string) error {
	_, err := c.Sender.To(peer).Edit(id).Text(ctx, text)
	return err
}

// DeleteMessage removes a message posted by this account.
func (c *Client) DeleteMessage(ctx context.Context, peer tg.InputPeerClass, id int) error {
	_, err := c.Sender.To(peer).Revoke().Messages(ctx, id)
	return err
}

// SendMediaFile uploads path and sends it into peer with the right media type,
// returning the new message id.
func (c *Client) SendMediaFile(ctx context.Context, peer tg.InputPeerClass, replyTo int, res DownloadResult, caption string) (int, error) {
	rb := c.Sender.To(peer)
	b := &rb.Builder
	if replyTo > 0 {
		b = rb.Reply(replyTo)
	}

	cap := []styling.StyledTextOption{styling.Plain(caption)}

	file, err := b.Upload(message.FromPath(res.Path)).AsInputFile(ctx)
	if err != nil {
		return 0, err
	}

	switch res.Kind {
	case KindPhoto:
		return unpack.MessageID(b.Media(ctx, message.UploadedPhoto(file, cap...)))
	case KindVideo:
		doc := message.UploadedDocument(file, cap...).
			Filename(res.Filename).
			MIME("video/mp4")
		if res.VideoAttr != nil {
			doc = doc.Attributes(res.VideoAttr)
		} else {
			doc = doc.Attributes(&tg.DocumentAttributeVideo{SupportsStreaming: true})
		}
		return unpack.MessageID(b.Media(ctx, doc))
	default:
		doc := message.UploadedDocument(file, cap...).
			Filename(res.Filename).
			ForceFile(true)
		return unpack.MessageID(b.Media(ctx, doc))
	}
}

// Forward copies message id from one peer to another (dropping author) and
// returns the new message id.
func (c *Client) Forward(ctx context.Context, from tg.InputPeerClass, id int, to tg.InputPeerClass) (int, error) {
	return unpack.MessageID(c.Sender.To(to).ForwardIDs(from, id).DropAuthor().Send(ctx))
}
