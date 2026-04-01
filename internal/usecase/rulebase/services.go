package rulebase

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/alipbudiman/wa-ai-agent.git/internal/domain/ports"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	errNilMessageEvent  = errors.New("message event is nil")
	errIgnoreOwnMessage = errors.New("ignore own message")
	errEmptyTextMessage = errors.New("message does not contain text content")
)

type RuleBase struct {
	responder ports.AgentResponder
}

func NewRuleBase(responder ports.AgentResponder) (*RuleBase, error) {
	if responder == nil {
		return nil, errors.New("agent responder is required")
	}

	return &RuleBase{responder: responder}, nil
}

func (r *RuleBase) handleMessage(v *events.Message) (string, error) {
	if v == nil || v.Message == nil {
		return "", errNilMessageEvent
	}
	if v.Info.IsFromMe {
		return "", errIgnoreOwnMessage
	}

	conversationText := strings.TrimSpace(v.Message.GetConversation())
	if conversationText != "" {
		return conversationText, nil
	}

	extendedText := strings.TrimSpace(v.Message.GetExtendedTextMessage().GetText())
	if extendedText != "" {
		return extendedText, nil
	}

	return "", errEmptyTextMessage
}

func (h *RuleBase) HandleMessageEvent(v *events.Message) {
	if h == nil {
		log.Printf("rule base is not initialized")
		return
	}

	text, err := h.handleMessage(v)

	if err != nil {
		if errors.Is(err, errIgnoreOwnMessage) || errors.Is(err, errEmptyTextMessage) {
			return
		}
		log.Printf("failed to handle incoming message: %v", err)
		return
	}

	log.Printf("incoming message from %s: %s", v.Info.Sender.String(), text)

	if h.responder == nil {
		log.Printf("responder is not initialized")
		return
	}

		if err := h.responder.SendText(context.Background(), v.Info.Chat.String(), "🤖_Processing to AI Agent, Please Wait.._"); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}


	replyText, err := h.responder.Reply(context.Background(), text)
	if err != nil {
		log.Printf("failed to generate reply: %v", err)
		return
	}

	if err := h.responder.SendText(context.Background(), v.Info.Chat.String(), replyText); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}

	log.Printf("reply sent to %s", v.Info.Chat.String())
}
