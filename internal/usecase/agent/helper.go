package rulebase

import (
	"context"
	"log"
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	ai_providers_api "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
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
			args.Text,
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
