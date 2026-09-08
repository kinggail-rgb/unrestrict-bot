package config

import (
	"strings"
	"testing"
)

func TestLoadRunRequiresAPICreds(t *testing.T) {
	for _, k := range []string{"API_ID", "API_HASH", "SESSION_STRING", "STRING_SESSION", "BOT_TOKEN", "CONTROL_CHAT"} {
		t.Setenv(k, "")
	}
	_, err := Load(ModeRun)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"API_ID is required", "API_HASH is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "SESSION_STRING") {
		t.Errorf("SESSION_STRING should be optional: %v", err)
	}
}

func TestLoadRunOK(t *testing.T) {
	t.Setenv("API_ID", "123")
	t.Setenv("API_HASH", "hash")
	t.Setenv("SESSION_STRING", "")
	t.Setenv("STRING_SESSION", "")
	t.Setenv("BOT_TOKEN", "")

	c, err := Load(ModeRun)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIID != 123 || c.APIHash != "hash" {
		t.Fatalf("bad config: %+v", c)
	}
	if c.ControlChat != "me" {
		t.Errorf("ControlChat default = %q, want me", c.ControlChat)
	}
	if c.BotEnabled() {
		t.Error("bot should be disabled")
	}
}

func TestLoadGenSessionOnlyNeedsAPICreds(t *testing.T) {
	t.Setenv("API_ID", "1")
	t.Setenv("API_HASH", "h")
	t.Setenv("SESSION_STRING", "")
	t.Setenv("STRING_SESSION", "")
	if _, err := Load(ModeGenSession); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadForwardRequiresPeers(t *testing.T) {
	t.Setenv("API_ID", "1")
	t.Setenv("API_HASH", "h")
	t.Setenv("FORWARD_FROM", "")
	t.Setenv("FORWARD_TO", "")
	_, err := Load(ModeForward)
	if err == nil || !strings.Contains(err.Error(), "FORWARD_FROM is required") {
		t.Fatalf("expected FORWARD_FROM error, got %v", err)
	}
}
