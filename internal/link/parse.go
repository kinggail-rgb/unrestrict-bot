// Package link parses Telegram links out of message text: private invite links
// (t.me/+hash, t.me/joinchat/hash), private message links (t.me/c/id/msg) and
// public message links (t.me/username/msg).
package link

import (
	"regexp"
	"strconv"
	"strings"
)

// Kind classifies a parsed link.
type Kind int

const (
	KindUnknown Kind = iota
	KindInvite
	KindMessage
)

// Link is a parsed Telegram link.
type Link struct {
	Kind Kind

	// InviteHash is set for KindInvite.
	InviteHash string

	// For KindMessage, exactly one of Username / ChannelID is set.
	Username  string
	ChannelID int64 // bare (positive) id, from t.me/c/ links
	MessageID int

	Raw string
}

var urlRe = regexp.MustCompile(`https?://t\.me/[^\s)>\]]+`)

// Find returns the first recognised link in text, or a Link with Kind
// KindUnknown if none is found. Additional pre-extracted URLs (e.g. from
// message entities) may be supplied and are checked first.
func Find(text string, extraURLs ...string) Link {
	seen := map[string]struct{}{}
	try := func(u string) (Link, bool) {
		u = strings.TrimSpace(strings.Trim(u, ".,;!?"))
		if u == "" {
			return Link{}, false
		}
		if _, dup := seen[u]; dup {
			return Link{}, false
		}
		seen[u] = struct{}{}
		if l := Parse(u); l.Kind != KindUnknown {
			return l, true
		}
		return Link{}, false
	}

	for _, u := range extraURLs {
		if l, ok := try(u); ok {
			return l
		}
	}
	for _, u := range urlRe.FindAllString(text, -1) {
		if l, ok := try(u); ok {
			return l
		}
	}
	return Link{Kind: KindUnknown, Raw: text}
}

// Parse parses a single URL string.
func Parse(raw string) Link {
	u := strings.TrimSpace(raw)
	l := Link{Raw: u}

	lower := strings.ToLower(u)
	if !strings.Contains(lower, "t.me/") {
		return l
	}

	if i := strings.Index(u, "t.me/+"); i >= 0 {
		if h := sanitizeHash(u[i+len("t.me/+"):]); h != "" {
			l.Kind = KindInvite
			l.InviteHash = h
		}
		return l
	}
	if i := strings.Index(u, "t.me/joinchat/"); i >= 0 {
		if h := sanitizeHash(u[i+len("t.me/joinchat/"):]); h != "" {
			l.Kind = KindInvite
			l.InviteHash = h
		}
		return l
	}

	i := strings.Index(u, "t.me/")
	path := u[i+len("t.me/"):]
	if q := strings.IndexAny(path, "?#"); q >= 0 {
		path = path[:q]
	}
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	// t.me/c/<id>/<msg> (a trailing thread id is ignored).
	if len(parts) >= 3 && parts[0] == "c" {
		id, err1 := strconv.ParseInt(parts[1], 10, 64)
		msg, err2 := strconv.Atoi(parts[len(parts)-1])
		if err1 == nil && err2 == nil {
			l.Kind = KindMessage
			l.ChannelID = id
			l.MessageID = msg
		}
		return l
	}

	// t.me/<username>/<msg>
	if len(parts) >= 2 {
		msg, err := strconv.Atoi(parts[len(parts)-1])
		if err == nil && parts[0] != "" {
			l.Kind = KindMessage
			l.Username = parts[0]
			l.MessageID = msg
		}
	}
	return l
}

func sanitizeHash(s string) string {
	if q := strings.IndexAny(s, "?#/"); q >= 0 {
		s = s[:q]
	}
	return strings.TrimSpace(s)
}
