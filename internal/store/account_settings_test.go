package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAccountSettingsCRUD(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	val, ok, err := db.GetAccountSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetAccountSetting error = %v", err)
	}
	if ok || val != "" {
		t.Fatalf("expected ok=false, val=\"\", got ok=%v, val=%q", ok, val)
	}

	if err := db.SetAccountSetting("test_key", "test_value"); err != nil {
		t.Fatalf("SetAccountSetting: %v", err)
	}

	val, ok, err = db.GetAccountSetting("test_key")
	if err != nil {
		t.Fatalf("GetAccountSetting error = %v", err)
	}
	if !ok || val != "test_value" {
		t.Fatalf("expected ok=true, val=\"test_value\", got ok=%v, val=%q", ok, val)
	}

	if err := db.SetAccountSetting("test_key", "updated_value"); err != nil {
		t.Fatalf("SetAccountSetting update: %v", err)
	}

	val, ok, err = db.GetAccountSetting("test_key")
	if err != nil {
		t.Fatalf("GetAccountSetting error = %v", err)
	}
	if !ok || val != "updated_value" {
		t.Fatalf("expected updated value, got %q", val)
	}

	if err := db.SetAccountSetting("", "val"); err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
	if _, _, err := db.GetAccountSetting(""); err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

func TestUnarchiveChatsSettingPermitted(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Default when unset: auto-unarchive permitted (Keep chats archived = OFF)
	permitted, err := db.UnarchiveChatsSettingPermitted()
	if err != nil {
		t.Fatalf("UnarchiveChatsSettingPermitted default error = %v", err)
	}
	if !permitted {
		t.Fatal("expected default unarchive setting to be permitted (true)")
	}

	// Disable auto-unarchive (Keep chats archived = ON)
	if err := db.SetUnarchiveChatsSetting(false); err != nil {
		t.Fatalf("SetUnarchiveChatsSetting(false): %v", err)
	}
	permitted, err = db.UnarchiveChatsSettingPermitted()
	if err != nil {
		t.Fatalf("UnarchiveChatsSettingPermitted after disable error = %v", err)
	}
	if permitted {
		t.Fatal("expected unarchive setting to be forbidden (false)")
	}

	// Enable auto-unarchive
	if err := db.SetUnarchiveChatsSetting(true); err != nil {
		t.Fatalf("SetUnarchiveChatsSetting(true): %v", err)
	}
	permitted, err = db.UnarchiveChatsSettingPermitted()
	if err != nil {
		t.Fatalf("UnarchiveChatsSettingPermitted after enable error = %v", err)
	}
	if !permitted {
		t.Fatal("expected unarchive setting to be permitted (true)")
	}
}

func TestUnarchiveChatIfPermitted(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	chat := "chat@s.whatsapp.net"
	if err := db.UpsertChat(chat, "dm", "Alice", time.Now()); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := db.SetChatArchived(chat, true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	c, err := db.GetChat(chat)
	if err != nil || !c.Archived {
		t.Fatalf("chat archived = %v, err = %v, want true", c.Archived, err)
	}

	// When permitted, unarchives chat
	if err := db.UnarchiveChatIfPermitted(chat); err != nil {
		t.Fatalf("UnarchiveChatIfPermitted: %v", err)
	}

	c, err = db.GetChat(chat)
	if err != nil || c.Archived {
		t.Fatalf("chat archived = %v, err = %v, want false after auto-unarchive", c.Archived, err)
	}

	// When setting is disabled, does not unarchive
	if err := db.SetUnarchiveChatsSetting(false); err != nil {
		t.Fatalf("SetUnarchiveChatsSetting(false): %v", err)
	}
	if err := db.SetChatArchived(chat, true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}
	if err := db.UnarchiveChatIfPermitted(chat); err != nil {
		t.Fatalf("UnarchiveChatIfPermitted when disabled: %v", err)
	}

	c, err = db.GetChat(chat)
	if err != nil || !c.Archived {
		t.Fatalf("chat archived = %v, err = %v, want true when unarchive disabled", c.Archived, err)
	}

	// Empty and non-existent chat safety
	if err := db.UnarchiveChatIfPermitted(""); err != nil {
		t.Fatalf("UnarchiveChatIfPermitted empty string: %v", err)
	}
	if err := db.UnarchiveChatIfPermitted("unknown@s.whatsapp.net"); err != nil {
		t.Fatalf("UnarchiveChatIfPermitted non-existent: %v", err)
	}
}

func TestUpsertMessageAutoUnarchivesIncoming(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	chat := "chat@s.whatsapp.net"
	base := time.Now().Truncate(time.Second)

	if err := db.UpsertChat(chat, "dm", "Alice", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := db.SetChatArchived(chat, true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	// Outgoing message (from_me = true) preserves archived state
	if err := db.UpsertMessage(UpsertMessageParams{
		ChatJID:   chat,
		MsgID:     "msg-out-1",
		Timestamp: base.Add(time.Second),
		FromMe:    true,
		Text:      "my response",
	}); err != nil {
		t.Fatalf("UpsertMessage outgoing: %v", err)
	}

	c, err := db.GetChat(chat)
	if err != nil || !c.Archived {
		t.Fatalf("chat archived = %v, want true after outgoing message", c.Archived)
	}

	// Incoming message (from_me = false) clears archived state when permitted
	if err := db.UpsertMessage(UpsertMessageParams{
		ChatJID:   chat,
		MsgID:     "msg-in-1",
		Timestamp: base.Add(2 * time.Second),
		FromMe:    false,
		Text:      "incoming message",
	}); err != nil {
		t.Fatalf("UpsertMessage incoming: %v", err)
	}

	c, err = db.GetChat(chat)
	if err != nil || c.Archived {
		t.Fatalf("chat archived = %v, want false after incoming message", c.Archived)
	}

	// When setting disables unarchive, incoming message preserves archived state
	if err := db.SetUnarchiveChatsSetting(false); err != nil {
		t.Fatalf("SetUnarchiveChatsSetting(false): %v", err)
	}
	if err := db.SetChatArchived(chat, true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	if err := db.UpsertMessage(UpsertMessageParams{
		ChatJID:   chat,
		MsgID:     "msg-in-2",
		Timestamp: base.Add(3 * time.Second),
		FromMe:    false,
		Text:      "another incoming message",
	}); err != nil {
		t.Fatalf("UpsertMessage incoming with setting disabled: %v", err)
	}

	c, err = db.GetChat(chat)
	if err != nil || !c.Archived {
		t.Fatalf("chat archived = %v, want true after incoming message with unarchive disabled", c.Archived)
	}
}

func TestUpsertMessageArchivedChatVisibleContentGating(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	chat := "chat-gating@s.whatsapp.net"
	base := time.Now().Truncate(time.Second)

	if err := db.UpsertChat(chat, "dm", "Contact", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	cases := []struct {
		name         string
		params       UpsertMessageParams
		wantArchived bool
	}{
		{
			name: "content-less placeholder row leaves chat archived",
			params: UpsertMessageParams{
				MsgID:       "msg-empty",
				DisplayText: "(message)",
				FromMe:      false,
			},
			wantArchived: true,
		},
		{
			name: "reaction leaves chat archived",
			params: UpsertMessageParams{
				MsgID:         "msg-react",
				ReactionToID:  "target-msg-1",
				ReactionEmoji: "❤️",
				FromMe:        false,
			},
			wantArchived: true,
		},
		{
			name: "revocation leaves chat archived",
			params: UpsertMessageParams{
				MsgID:   "msg-revoked",
				Revoked: true,
				FromMe:  false,
			},
			wantArchived: true,
		},
		{
			name: "media without caption unarchives chat",
			params: UpsertMessageParams{
				MsgID:     "msg-media",
				MediaType: "image",
				FromMe:    false,
			},
			wantArchived: false,
		},
		{
			name: "plain text unarchives chat",
			params: UpsertMessageParams{
				MsgID:  "msg-text",
				Text:   "Hello there",
				FromMe: false,
			},
			wantArchived: false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.SetChatArchived(chat, true); err != nil {
				t.Fatalf("SetChatArchived: %v", err)
			}
			p := tc.params
			p.ChatJID = chat
			p.Timestamp = base.Add(time.Duration(i+1) * time.Minute)
			if err := db.UpsertMessage(p); err != nil {
				t.Fatalf("UpsertMessage: %v", err)
			}
			c, err := db.GetChat(chat)
			if err != nil {
				t.Fatalf("GetChat: %v", err)
			}
			if c.Archived != tc.wantArchived {
				t.Fatalf("%s: archived = %v, want %v", tc.name, c.Archived, tc.wantArchived)
			}
		})
	}
}

