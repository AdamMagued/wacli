package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openclaw/wacli/internal/wa"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestLiveSyncMessageUnarchivesArchivedChatByDefault(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "12345", Server: types.DefaultUserServer}
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := a.db.UpsertChat(chat.String(), "dm", "Alice", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := a.db.SetChatArchived(chat.String(), true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	c, err := a.db.GetChat(chat.String())
	if err != nil || !c.Archived {
		t.Fatalf("precondition: chat archived = %v, err = %v", c.Archived, err)
	}

	// Incoming message (IsFromMe: false)
	incoming := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
			},
			ID:        "msg-incoming-1",
			Timestamp: base.Add(time.Minute),
			PushName:  "Alice",
		},
		Message: &waProto.Message{Conversation: proto.String("Hey, are you there?")},
	}

	var messagesStored atomic.Int64
	a.handleLiveSyncMessage(context.Background(), SyncOptions{}, incoming, &messagesStored, func(string, string) {}, nil)

	if messagesStored.Load() != 1 {
		t.Fatalf("expected 1 message stored, got %d", messagesStored.Load())
	}

	c, err = a.db.GetChat(chat.String())
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if c.Archived {
		t.Fatalf("expected chat to be unarchived after incoming message, got archived = %v", c.Archived)
	}
}

func TestLiveSyncOutgoingMessagePreservesArchivedChat(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "12345", Server: types.DefaultUserServer}
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := a.db.UpsertChat(chat.String(), "dm", "Alice", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := a.db.SetChatArchived(chat.String(), true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	// Outgoing message (IsFromMe: true)
	outgoing := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: true,
			},
			ID:        "msg-outgoing-1",
			Timestamp: base.Add(time.Minute),
			PushName:  "Me",
		},
		Message: &waProto.Message{Conversation: proto.String("I am sending this")},
	}

	var messagesStored atomic.Int64
	a.handleLiveSyncMessage(context.Background(), SyncOptions{}, outgoing, &messagesStored, func(string, string) {}, nil)

	c, err := a.db.GetChat(chat.String())
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if !c.Archived {
		t.Fatalf("expected chat to remain archived after outgoing message, got archived = %v", c.Archived)
	}
}

func TestLiveSyncMessagePreservesArchivedChatWhenSettingDisabled(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "12345", Server: types.DefaultUserServer}
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := a.db.UpsertChat(chat.String(), "dm", "Alice", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := a.db.SetChatArchived(chat.String(), true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	// Disable auto-unarchive via app-state event (Keep chats archived = ON)
	err := a.handleChatStateEvent(context.Background(), &events.UnarchiveChatsSetting{
		Action: &waSyncAction.UnarchiveChatsSetting{
			UnarchiveChats: proto.Bool(false),
		},
	})
	if err != nil {
		t.Fatalf("handleChatStateEvent: %v", err)
	}

	permitted, err := a.db.UnarchiveChatsSettingPermitted()
	if err != nil || permitted {
		t.Fatalf("expected unarchive setting permitted=false, got %v, err=%v", permitted, err)
	}

	incoming := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
			},
			ID:        "msg-incoming-disabled",
			Timestamp: base.Add(time.Minute),
			PushName:  "Alice",
		},
		Message: &waProto.Message{Conversation: proto.String("Still here")},
	}

	var messagesStored atomic.Int64
	a.handleLiveSyncMessage(context.Background(), SyncOptions{}, incoming, &messagesStored, func(string, string) {}, nil)

	c, err := a.db.GetChat(chat.String())
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if !c.Archived {
		t.Fatalf("expected chat to stay archived when unarchive setting is disabled, got archived = %v", c.Archived)
	}
}

func TestGroupIncomingMessageUnarchivesArchivedGroup(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	groupJID := types.JID{User: "120363001@g.us", Server: types.GroupServer}
	senderJID := types.JID{User: "999", Server: types.DefaultUserServer}
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := a.db.UpsertChat(groupJID.String(), "group", "Work Group", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := a.db.SetChatArchived(groupJID.String(), true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	c, err := a.db.GetChat(groupJID.String())
	if err != nil || !c.Archived {
		t.Fatalf("precondition: group archived = %v, err = %v", c.Archived, err)
	}

	incoming := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     groupJID,
				Sender:   senderJID,
				IsFromMe: false,
				IsGroup:  true,
			},
			ID:        "grp-msg-1",
			Timestamp: base.Add(time.Minute),
			PushName:  "Colleague",
		},
		Message: &waProto.Message{Conversation: proto.String("Meeting now")},
	}

	var messagesStored atomic.Int64
	a.handleLiveSyncMessage(context.Background(), SyncOptions{}, incoming, &messagesStored, func(string, string) {}, nil)

	c, err = a.db.GetChat(groupJID.String())
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if c.Archived {
		t.Fatalf("expected group chat to be unarchived after incoming group message, got archived = %v", c.Archived)
	}
}

func TestHistorySyncStoresGlobalAutoUnarchiveSetting(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "12345", Server: types.DefaultUserServer}
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := a.db.UpsertChat(chat.String(), "dm", "Alice", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := a.db.SetChatArchived(chat.String(), true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	// History sync event with GlobalSettings.AutoUnarchiveChats = false
	histSyncEvt := &events.HistorySync{
		Data: &waHistorySync.HistorySync{
			SyncType: waHistorySync.HistorySync_RECENT.Enum(),
			GlobalSettings: &waHistorySync.GlobalSettings{
				AutoUnarchiveChats: proto.Bool(false),
			},
			Conversations: []*waHistorySync.Conversation{
				{
					ID: proto.String(chat.String()),
				},
			},
		},
	}

	var messagesStored, lastEvent atomic.Int64
	a.handleHistorySync(context.Background(), SyncOptions{}, histSyncEvt, &messagesStored, &lastEvent, func(string, string) {})

	permitted, err := a.db.UnarchiveChatsSettingPermitted()
	if err != nil || permitted {
		t.Fatalf("expected unarchive permitted = false from history sync, got %v, err = %v", permitted, err)
	}

	// Incoming message stored for sync
	err = a.storeParsedMessage(context.Background(), wa.ParsedMessage{
		Chat:      chat,
		ID:        "hist-msg-1",
		Timestamp: base.Add(2 * time.Minute),
		FromMe:    false,
		Text:      "Should not unarchive because setting is false",
	})
	if err != nil {
		t.Fatalf("storeParsedMessage: %v", err)
	}

	c, err := a.db.GetChat(chat.String())
	if err != nil || !c.Archived {
		t.Fatalf("expected chat to stay archived when global setting disabled unarchive, got archived = %v", c.Archived)
	}
}

func TestStoreParsedMessageUnarchivesArchivedChat(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "54321", Server: types.DefaultUserServer}
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := a.db.UpsertChat(chat.String(), "dm", "Bob", base); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := a.db.SetChatArchived(chat.String(), true); err != nil {
		t.Fatalf("SetChatArchived: %v", err)
	}

	// Outgoing does not unarchive
	err := a.storeParsedMessage(context.Background(), wa.ParsedMessage{
		Chat:      chat,
		ID:        "msg-out-direct",
		Timestamp: base.Add(time.Second),
		FromMe:    true,
		Text:      "outgoing",
	})
	if err != nil {
		t.Fatalf("storeParsedMessage outgoing: %v", err)
	}

	c, err := a.db.GetChat(chat.String())
	if err != nil || !c.Archived {
		t.Fatalf("expected chat to remain archived after outgoing storeParsedMessage, got %v", c.Archived)
	}

	// Incoming unarchives
	err = a.storeParsedMessage(context.Background(), wa.ParsedMessage{
		Chat:      chat,
		ID:        "msg-in-direct",
		Timestamp: base.Add(2 * time.Second),
		FromMe:    false,
		Text:      "incoming",
	})
	if err != nil {
		t.Fatalf("storeParsedMessage incoming: %v", err)
	}

	c, err = a.db.GetChat(chat.String())
	if err != nil || c.Archived {
		t.Fatalf("expected chat to be unarchived after incoming storeParsedMessage, got %v", c.Archived)
	}
}

func TestArchivedGroupIncomingVisibleContentGating(t *testing.T) {
	cases := []struct {
		name         string
		msg          *waProto.Message
		wantArchived bool
	}{
		{
			name:         "text (control)",
			msg:          &waProto.Message{Conversation: proto.String("Meeting now")},
			wantArchived: false,
		},
		{
			name: "senderKeyDistribution only",
			msg: &waProto.Message{
				SenderKeyDistributionMessage: &waProto.SenderKeyDistributionMessage{
					GroupID: proto.String("120363001@g.us"),
				},
			},
			wantArchived: true,
		},
		{
			name: "reaction only",
			msg: &waProto.Message{
				ReactionMessage: &waProto.ReactionMessage{
					Key:  &waProto.MessageKey{ID: proto.String("target-1"), FromMe: proto.Bool(true)},
					Text: proto.String("👍"),
				},
			},
			wantArchived: true,
		},
		{
			name: "image media with caption",
			msg: &waProto.Message{
				ImageMessage: &waProto.ImageMessage{
					Caption: proto.String("Agenda photo"),
				},
			},
			wantArchived: false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t)
			a.wa = newFakeWA()

			group := types.JID{User: "120363001", Server: types.GroupServer}
			sender := types.JID{User: "999", Server: types.DefaultUserServer}
			base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

			if err := a.db.UpsertChat(group.String(), "group", "Work Group", base); err != nil {
				t.Fatalf("UpsertChat: %v", err)
			}
			if err := a.db.SetChatArchived(group.String(), true); err != nil {
				t.Fatalf("SetChatArchived: %v", err)
			}

			evt := &events.Message{
				Info: types.MessageInfo{
					MessageSource: types.MessageSource{Chat: group, Sender: sender, IsGroup: true},
					ID:            "probe-" + string(rune('a'+i)),
					Timestamp:     base.Add(time.Minute),
				},
				Message: tc.msg,
			}

			var stored atomic.Int64
			a.handleLiveSyncMessage(context.Background(), SyncOptions{}, evt, &stored, func(string, string) {}, nil)

			c, err := a.db.GetChat(group.String())
			if err != nil {
				t.Fatalf("GetChat: %v", err)
			}
			if c.Archived != tc.wantArchived {
				t.Fatalf("%s: archived=%v, want %v", tc.name, c.Archived, tc.wantArchived)
			}
		})
	}
}

