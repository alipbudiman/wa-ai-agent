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

func (h *Agents) isFromGroup(v *events.Message) string {
	sender := v.Info.Chat.Server
	if sender == types.GroupServer {
		return v.Info.Chat.String() // using Chat ID as group identifier
	}
	return ""
}
