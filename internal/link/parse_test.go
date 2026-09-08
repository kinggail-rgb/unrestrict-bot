package link

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Link
	}{
		{
			name: "invite plus",
			in:   "https://t.me/+AbCdEf12345",
			want: Link{Kind: KindInvite, InviteHash: "AbCdEf12345"},
		},
		{
			name: "invite joinchat",
			in:   "https://t.me/joinchat/AbCdEf12345",
			want: Link{Kind: KindInvite, InviteHash: "AbCdEf12345"},
		},
		{
			name: "invite plus with trailing query",
			in:   "https://t.me/+AbCdEf12345?foo=bar",
			want: Link{Kind: KindInvite, InviteHash: "AbCdEf12345"},
		},
		{
			name: "private channel message",
			in:   "https://t.me/c/1234567890/42",
			want: Link{Kind: KindMessage, ChannelID: 1234567890, MessageID: 42},
		},
		{
			name: "private channel message with thread",
			in:   "https://t.me/c/1234567890/99/42",
			want: Link{Kind: KindMessage, ChannelID: 1234567890, MessageID: 42},
		},
		{
			name: "public message",
			in:   "https://t.me/somechannel/123",
			want: Link{Kind: KindMessage, Username: "somechannel", MessageID: 123},
		},
		{
			name: "public message with query",
			in:   "https://t.me/somechannel/123?single",
			want: Link{Kind: KindMessage, Username: "somechannel", MessageID: 123},
		},
		{
			name: "not a link",
			in:   "hello world",
			want: Link{Kind: KindUnknown},
		},
		{
			name: "t.me profile without message id",
			in:   "https://t.me/somechannel",
			want: Link{Kind: KindUnknown},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.in)
			if got.Kind != tt.want.Kind || got.InviteHash != tt.want.InviteHash ||
				got.Username != tt.want.Username || got.ChannelID != tt.want.ChannelID ||
				got.MessageID != tt.want.MessageID {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	l := Find("please grab https://t.me/somechannel/123 thanks")
	if l.Kind != KindMessage || l.Username != "somechannel" || l.MessageID != 123 {
		t.Fatalf("Find returned %+v", l)
	}

	l = Find("no link here")
	if l.Kind != KindUnknown {
		t.Fatalf("Find returned %+v, want unknown", l)
	}

	l = Find("text", "https://t.me/c/100/5")
	if l.Kind != KindMessage || l.ChannelID != 100 || l.MessageID != 5 {
		t.Fatalf("Find(extra) returned %+v", l)
	}
}
