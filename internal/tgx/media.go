package tgx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kamuridesu/unrestrict-bot/internal/link"

	"github.com/gotd/td/tg"
)

// MediaKind classifies how media should be re-sent.
type MediaKind int

const (
	KindNone MediaKind = iota
	KindPhoto
	KindVideo
	KindDocument
)

// SourceMessage is a resolved target message plus its origin peer.
type SourceMessage struct {
	Msg  *tg.Message
	Peer tg.InputPeerClass
}

// FetchMessage resolves the source peer for a message link and returns the
// target message. A nil message (with nil error) means the message or chat is
// gone or not visible.
func (c *Client) FetchMessage(ctx context.Context, l link.Link) (*SourceMessage, error) {
	if l.Kind != link.KindMessage {
		return nil, fmt.Errorf("not a message link")
	}

	var (
		inputPeer tg.InputPeerClass
		res       tg.MessagesMessagesClass
		err       error
	)

	if l.Username != "" {
		p, rerr := c.Peers.Resolve(ctx, l.Username)
		if rerr != nil {
			return nil, nil
		}
		inputPeer = p.InputPeer()
		if ch, ok := p.(interface{ InputChannel() tg.InputChannelClass }); ok {
			res, err = c.API.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: ch.InputChannel(),
				ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: l.MessageID}},
			})
		} else {
			res, err = c.API.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: l.MessageID}})
		}
	} else {
		ch, rerr := c.Peers.ResolveChannelID(ctx, l.ChannelID)
		if rerr != nil {
			return nil, nil
		}
		inputPeer = ch.InputPeer()
		res, err = c.API.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: ch.InputChannel(),
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: l.MessageID}},
		})
	}
	if err != nil {
		return nil, err
	}

	mod, ok := res.AsModified()
	if !ok {
		return nil, nil
	}
	for _, m := range mod.GetMessages() {
		if msg, ok := m.(*tg.Message); ok && msg.ID == l.MessageID {
			return &SourceMessage{Msg: msg, Peer: inputPeer}, nil
		}
	}
	return nil, nil
}

// DownloadResult describes a downloaded file.
type DownloadResult struct {
	Path      string
	Kind      MediaKind
	Filename  string
	Size      int64
	VideoAttr *tg.DocumentAttributeVideo
}

// ProgressFunc is called with (downloaded, total) bytes.
type ProgressFunc func(done, total int64)

// Download saves the media of msg into dir and reports its kind. A message with
// no downloadable media returns Kind KindNone.
func (c *Client) Download(ctx context.Context, msg *tg.Message, dir string, progress ProgressFunc) (DownloadResult, error) {
	var (
		loc       tg.InputFileLocationClass
		size      int64
		kind      MediaKind
		filename  string
		videoAttr *tg.DocumentAttributeVideo
	)

	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		if !ok {
			return DownloadResult{}, nil
		}
		typ := largestPhotoSize(photo)
		loc = photo.AsInputPhotoFileLocation(typ)
		size = photoSizeBytes(photo, typ)
		kind = KindPhoto
		filename = fmt.Sprintf("photo_%d.jpg", photo.ID)

	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return DownloadResult{}, nil
		}
		loc = doc.AsInputDocumentFileLocation("")
		size = doc.Size
		filename = documentFilename(doc)
		if isVideo(doc) {
			kind = KindVideo
			for _, attr := range doc.Attributes {
				if v, ok := attr.(*tg.DocumentAttributeVideo); ok {
					videoAttr = v
				}
			}
		} else {
			kind = KindDocument
		}

	default:
		return DownloadResult{}, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DownloadResult{}, err
	}
	f, err := os.CreateTemp(dir, "unrestrict-*"+filepath.Ext(filename))
	if err != nil {
		return DownloadResult{}, err
	}
	path := f.Name()

	w := &countingWriterAt{w: f, total: size, cb: progress}
	_, derr := c.Downloader.Download(c.API, loc).WithThreads(4).Parallel(ctx, w)
	closeErr := f.Close()
	if derr != nil {
		_ = os.Remove(path)
		return DownloadResult{}, derr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return DownloadResult{}, closeErr
	}

	if size == 0 {
		if st, serr := os.Stat(path); serr == nil {
			size = st.Size()
		}
	}
	return DownloadResult{Path: path, Kind: kind, Filename: filename, Size: size, VideoAttr: videoAttr}, nil
}

// MediaKindOf reports how a message's media should be re-sent without
// downloading it.
func MediaKindOf(msg *tg.Message) MediaKind {
	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		return KindPhoto
	case *tg.MessageMediaDocument:
		if doc, ok := media.Document.(*tg.Document); ok && isVideo(doc) {
			return KindVideo
		}
		return KindDocument
	default:
		return KindNone
	}
}

func isVideo(doc *tg.Document) bool {
	if strings.EqualFold(doc.MimeType, "video/mp4") {
		return true
	}
	for _, attr := range doc.Attributes {
		if _, ok := attr.(*tg.DocumentAttributeVideo); ok {
			return true
		}
	}
	return false
}

func documentFilename(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		if fn, ok := attr.(*tg.DocumentAttributeFilename); ok && fn.FileName != "" {
			return fn.FileName
		}
	}
	ext := ""
	switch {
	case strings.EqualFold(doc.MimeType, "video/mp4"):
		ext = ".mp4"
	case strings.HasPrefix(doc.MimeType, "image/"):
		ext = "." + strings.TrimPrefix(doc.MimeType, "image/")
	}
	return fmt.Sprintf("document_%d%s", doc.ID, ext)
}

func largestPhotoSize(photo *tg.Photo) string {
	best := ""
	bestArea := -1
	for _, s := range photo.Sizes {
		switch v := s.(type) {
		case *tg.PhotoSize:
			if a := v.W * v.H; a > bestArea {
				bestArea, best = a, v.Type
			}
		case *tg.PhotoSizeProgressive:
			if a := v.W * v.H; a > bestArea {
				bestArea, best = a, v.Type
			}
		}
	}
	if best == "" {
		best = "x"
	}
	return best
}

func photoSizeBytes(photo *tg.Photo, typ string) int64 {
	for _, s := range photo.Sizes {
		switch v := s.(type) {
		case *tg.PhotoSize:
			if v.Type == typ {
				return int64(v.Size)
			}
		case *tg.PhotoSizeProgressive:
			if v.Type == typ && len(v.Sizes) > 0 {
				return int64(v.Sizes[len(v.Sizes)-1])
			}
		}
	}
	return 0
}

// countingWriterAt wraps an io.WriterAt and reports progress at most every 3s.
// Download workers call WriteAt concurrently, so state is synchronised.
type countingWriterAt struct {
	w interface {
		WriteAt([]byte, int64) (int, error)
	}
	total int64
	cb    ProgressFunc

	written  atomic.Int64
	mu       sync.Mutex
	lastCall time.Time
}

func (c *countingWriterAt) WriteAt(p []byte, off int64) (int, error) {
	n, err := c.w.WriteAt(p, off)
	done := c.written.Add(int64(n))
	if c.cb != nil {
		c.mu.Lock()
		emit := time.Since(c.lastCall) > 3*time.Second
		if emit {
			c.lastCall = time.Now()
		}
		c.mu.Unlock()
		if emit {
			c.cb(done, c.total)
		}
	}
	return n, err
}
