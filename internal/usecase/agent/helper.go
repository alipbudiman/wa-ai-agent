package rulebase

import (
	"context"
	"log"
	"slices"
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	ai_providers_api "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	"github.com/sashabaranov/go-openai"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func (h *Agents) handleMessageEventGuard() bool {
	if h == nil {
		log.Printf("rule base is not initialized")
		return false
	}

	if h.responder == nil {
		log.Printf("responder is not initialized")
		return false
	}

	if h.waclient == nil {
		log.Printf("whatsapp client is not initialized")
		return false
	}

	return true
}

func (h *Agents) throwToAgent(args CommandArgs, Command string) (bool, error) {
	selected := strings.TrimSpace(strings.TrimPrefix(args.Text, Command))
	if selected != "" {
		replyStatus, err := h.responder.Reply(
			context.Background(),
			[]openai.ChatCompletionMessage{
				{
					Role:    h.isFromGroup(args.Evt),
					Content: args.Text,
				},
			},
			false,
			ports.NONE,
			args.Evt.Info.Sender,
		)
		if err != nil {
			return false, err
		}

		if replyStatus == ai_providers_api.SUCCESS {
			return true, nil
		}

		return false, ErrFailToProcessMessage
	}

	return false, nil
}

func (h *Agents) recordAgentHistory(ctx context.Context, jid types.JID, groupID string, content string) (string, error) {
	threadID := jid.ToNonAD().String()
	if groupID != "" {
		threadID = groupID
	}

	if h.chatCompleter != nil {
		success, err := h.chatCompleter.InjectAIConversation(ctx, content, threadID)
		if err != nil {
			return "", err
		}
		if success {
			return ai_providers_api.SUCCESS, nil
		}

		return "", ErrFailToProcessMessage
	}

	return h.responder.Reply(
		ctx,
		[]openai.ChatCompletionMessage{
			{
				Role:    groupID,
				Content: content,
			},
		},
		true,
		ports.HISTORY,
		jid,
	)
}

func recipientThreadGroupID(jid types.JID) string {
	if jid.Server == types.GroupServer {
		return jid.String()
	}

	return ""
}

func (h *Agents) isFromGroup(v *events.Message) string {
	sender := v.Info.Chat.Server
	if sender == types.GroupServer {
		return v.Info.Chat.String() // using Chat ID as group identifier
	}
	return ""
}

func (h *Agents) getReplyMessage(v *events.Message) string {
	if v == nil || v.RawMessage == nil {
		return ""
	}

	extended := v.RawMessage.GetExtendedTextMessage()
	if extended == nil {
		return ""
	}

	contextInfo := extended.GetContextInfo()
	if contextInfo == nil {
		return ""
	}

	quoted := contextInfo.GetQuotedMessage()
	if quoted == nil {
		return ""
	}

	if conversation := strings.TrimSpace(quoted.GetConversation()); conversation != "" {
		return conversation
	}

	if quotedExtended := quoted.GetExtendedTextMessage(); quotedExtended != nil {
		if text := strings.TrimSpace(quotedExtended.GetText()); text != "" {
			return text
		}
	}

	if quotedImage := quoted.GetImageMessage(); quotedImage != nil {
		if caption := strings.TrimSpace(quotedImage.GetCaption()); caption != "" {
			return caption
		}
	}

	if quotedVideo := quoted.GetVideoMessage(); quotedVideo != nil {
		if caption := strings.TrimSpace(quotedVideo.GetCaption()); caption != "" {
			return caption
		}
	}

	return ""
}

func getMention(v *events.Message) []string {
	newArry := []string{}
	Vm := v.Message
	if IsExtendedTextMessage := Vm.GetExtendedTextMessage(); IsExtendedTextMessage != nil {
		if IsContextInfo := IsExtendedTextMessage.GetContextInfo(); IsContextInfo != nil {
			if IsMentionedJid := IsContextInfo.GetMentionedJID(); IsMentionedJid != nil {
				if len(IsMentionedJid) > 0 {
					for _, x := range IsMentionedJid {
						if !slices.Contains(newArry, x) {
							newArry = append(newArry, x)
						}
					}
				}
			}
		}
	}
	return newArry
}
func getReply(v *events.Message) *GetReplyResponse {
	Vm := v.Message
	if IsExtendedTextMessage := Vm.GetExtendedTextMessage(); IsExtendedTextMessage != nil {
		if IsContextInfo := IsExtendedTextMessage.GetContextInfo(); IsContextInfo != nil {
			if IsQuotedMessage := IsContextInfo.GetQuotedMessage(); IsQuotedMessage != nil {
				var Chat string
				if IsConversation := IsQuotedMessage.GetConversation(); IsConversation != "" {
					Chat = IsConversation
				} else {
					Chat = IsQuotedMessage.ExtendedTextMessage.GetText()
				}
				ret := &GetReplyResponse{
					StanzaId:     IsContextInfo.GetStanzaID(),
					Participant:  IsContextInfo.GetParticipant(),
					Conversation: Chat,
					Message:      Vm,
				}
				return ret
			}
		}
	}
	return nil
}

func (h *Agents) resolveSender(v *events.Message) types.JID {
	if v == nil {
		return types.JID{}
	}
	resolvedSender := v.Info.Sender.ToNonAD()
	if !v.Info.SenderAlt.IsEmpty() && v.Info.SenderAlt.Server == types.DefaultUserServer {
		resolvedSender = v.Info.SenderAlt.ToNonAD()
	}
	return resolvedSender
}

// resolveBotJID returns the bot's own JID. It prefers the connected client's
// self JID and falls back to parsing the configured bot phone number.
func (h *Agents) resolveBotJID() (types.JID, error) {
	botJID, err := h.waclient.GetSelfJID(context.Background())
	if err == nil && !botJID.IsEmpty() {
		return botJID, nil
	}
	if err != nil {
		log.Printf("error getting bot self JID: %v", err)
	}

	botJID, err = h.waclient.ParseJID(h.config.BotInfo.Number)
	if err != nil {
		return types.JID{}, err
	}
	return botJID, nil
}

// groupMessageAddressesBot reports whether an incoming message is directed at
// the bot, which decides whether the bot should respond:
//   - Non-group (DM) chats always return true.
//   - Group chats return true only when the bot is @mentioned or the message
//     quotes/replies to the bot.
//   - It fails open (returns true) if the bot JID cannot be resolved, so a
//     resolution error never silently mutes the bot.
//
// JIDs are normalized via ToNonAD before comparing, so a device suffix
// (e.g. ":5@s.whatsapp.net") on the bot's self JID cannot mismatch the
// deviceless mention/reply JIDs.
func (h *Agents) groupMessageAddressesBot(v *events.Message) bool {
	if v == nil || v.Info.Chat.Server != types.GroupServer {
		return true
	}

	botJID, err := h.resolveBotJID()
	if err != nil {
		log.Printf("failed to resolve bot JID; failing open (will respond): %v", err)
		return true
	}
	botAddr := botJID.ToNonAD().String()

	if mentions := getMention(v); slices.Contains(mentions, botAddr) {
		return true
	}

	if replyData := getReply(v); replyData != nil && replyData.Participant != "" {
		participant, err := h.waclient.ParseJID(replyData.Participant)
		if err != nil {
			log.Printf("error parsing reply participant JID %q: %v", replyData.Participant, err)
		} else if participant.ToNonAD().String() == botAddr {
			return true
		}
	}

	return false
}

