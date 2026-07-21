package rulebase

import (
	"context"
	"strings"
	"testing"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// fakeWhatsAppClient embeds the WhatsAppClient interface so only the methods
// the helper under test touches (GetSelfJID, ParseJID, GetAltJID) need
// implementing. Any other method would panic on use, which is fine here.
type fakeWhatsAppClient struct {
	ports.WhatsAppClient
	self   types.JID
	lidMap map[string]string // ToNonAD string -> alternate ToNonAD string
}

func (f *fakeWhatsAppClient) GetSelfJID(context.Context) (types.JID, error) {
	return f.self, nil
}

// ParseJID mirrors production (resolver.go): "user@server" is parsed by
// whatsmeow (so @lid is preserved), a bare token is treated as a PN.
func (f *fakeWhatsAppClient) ParseJID(target string) (types.JID, error) {
	if strings.Contains(target, "@") {
		return types.ParseJID(target)
	}
	return types.NewJID(target, types.DefaultUserServer), nil
}

// GetAltJID mirrors the LID<->PN store lookup, backed by the test's lidMap.
func (f *fakeWhatsAppClient) GetAltJID(_ context.Context, jid types.JID) (types.JID, error) {
	if f.lidMap == nil {
		return types.JID{}, nil
	}
	if alt, ok := f.lidMap[jid.ToNonAD().String()]; ok {
		return types.ParseJID(alt)
	}
	return types.JID{}, nil
}

// buildMsg builds an incoming text message. When server is a group, optional
// @mentions and/or a reply (quoting) participant can be attached.
func buildMsg(chat types.JID, mentions []string, replyParticipant string) *events.Message {
	ext := &waE2E.ExtendedTextMessage{Text: proto.String("hi")}
	ci := &waE2E.ContextInfo{}
	if len(mentions) > 0 {
		ci.MentionedJID = mentions
	}
	if replyParticipant != "" {
		ci.Participant = proto.String(replyParticipant)
		ci.QuotedMessage = &waE2E.Message{Conversation: proto.String("bot previous message")}
	}
	ext.ContextInfo = ci

	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   chat,
				Sender: types.JID{User: "628000000000", Server: types.DefaultUserServer},
			},
		},
		Message: &waE2E.Message{ExtendedTextMessage: ext},
	}
}

func TestGroupMessageAddressesBot(t *testing.T) {
	botJID := types.JID{User: "628111222333", Server: types.DefaultUserServer}
	botAddr := botJID.ToNonAD().String() // "628111222333@s.whatsapp.net"
	otherAddr := "628999888777@s.whatsapp.net"
	botLID := "112417094811836@lid"
	otherLID := "299988877766@lid"

	group := types.JID{User: "120363group", Server: types.GroupServer}
	dm := types.JID{User: "628000000000", Server: types.DefaultUserServer}

	// Bot self JID carrying a device suffix, to prove comparison is normalized.
	botJIDWithDevice := types.JID{User: botJID.User, Server: types.DefaultUserServer, Device: 5}

	// LID<->PN mapping for the bot only (mirrors a populated device store).
	botLIDMap := map[string]string{botAddr: botLID}

	cases := []struct {
		name   string
		v      *events.Message
		self   types.JID
		lidMap map[string]string
		want   bool
	}{
		// Phone-number (PN) address space
		{"DM always proceeds", buildMsg(dm, nil, ""), botJID, nil, true},
		{"group with bot @mention (PN)", buildMsg(group, []string{botAddr}, ""), botJID, nil, true},
		{"group mentioning someone else (PN)", buildMsg(group, []string{otherAddr}, ""), botJID, nil, false},
		{"group with no mention and no reply", buildMsg(group, nil, ""), botJID, nil, false},
		{"group reply quoting the bot (PN)", buildMsg(group, nil, botAddr), botJID, nil, true},
		{"group reply quoting someone else (PN)", buildMsg(group, nil, otherAddr), botJID, nil, false},
		{"group mention matches despite bot device suffix", buildMsg(group, []string{botAddr}, ""), botJIDWithDevice, nil, true},
		{"nil message is treated as addressable (safe)", nil, botJID, nil, true},

		// Linked-identity (LID) address space
		{"group mention bot by LID", buildMsg(group, []string{botLID}, ""), botJID, botLIDMap, true},
		{"group reply quoting bot by LID", buildMsg(group, nil, botLID), botJID, botLIDMap, true},
		{"group mentioning someone else by LID", buildMsg(group, []string{otherLID}, ""), botJID, botLIDMap, false},
		{"group reply quoting someone else by LID", buildMsg(group, nil, otherLID), botJID, botLIDMap, false},
		// Graceful degradation: bot LID mapping unknown -> mention by LID not recognized (fail-closed).
		{"bot LID unknown stays fail-closed on LID mention", buildMsg(group, []string{botLID}, ""), botJID, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &Agents{
				waclient: &fakeWhatsAppClient{self: tc.self, lidMap: tc.lidMap},
				config:   config.Config{},
			}

			if got := agent.groupMessageAddressesBot(tc.v); got != tc.want {
				t.Fatalf("groupMessageAddressesBot = %v, want %v", got, tc.want)
			}
		})
	}
}
