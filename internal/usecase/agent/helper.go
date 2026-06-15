package rulebase

import (
	"context"
	"log"
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
