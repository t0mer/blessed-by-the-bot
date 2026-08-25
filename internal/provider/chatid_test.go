package provider_test

import (
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

func TestNormalizeChatIDFromPhone(t *testing.T) {
	cases := map[string]string{
		"972501234567":     "972501234567@c.us",
		"+972501234567":    "972501234567@c.us",
		"+972 50-123 4567": "972501234567@c.us",
		"(972) 50 1234567": "972501234567@c.us",
	}
	for in, want := range cases {
		got, err := provider.NormalizeChatID(in)
		if err != nil {
			t.Errorf("NormalizeChatID(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeChatID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeChatIDPassesThroughJIDs(t *testing.T) {
	for _, jid := range []string{"972501234567@c.us", "120363012345678901@g.us"} {
		got, err := provider.NormalizeChatID(jid)
		if err != nil {
			t.Errorf("NormalizeChatID(%q): %v", jid, err)
			continue
		}
		if got != jid {
			t.Errorf("NormalizeChatID(%q) = %q, want it unchanged", jid, got)
		}
	}
}

func TestNormalizeChatIDRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "   ", "abc", "12345", "1234567890123456789", "@c.us"} {
		if got, err := provider.NormalizeChatID(in); err == nil {
			t.Errorf("NormalizeChatID(%q) = %q, want an error", in, got)
		}
	}
}

func TestIsGroupChatID(t *testing.T) {
	if !provider.IsGroupChatID("120363012345678901@g.us") {
		t.Error("group JID not recognised")
	}
	if provider.IsGroupChatID("972501234567@c.us") {
		t.Error("private JID reported as a group")
	}
}
