package rulebase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	errNilMessageEvent  = errors.New("message event is nil")
	errIgnoreOwnMessage = errors.New("ignore own message")
	errEmptyTextMessage = errors.New("message does not contain text content")
)

type RuleBase struct {
	waclient  ports.WhatsAppClient
	responder ports.AgentResponder
	config    config.Config
}

func NewRuleBase(waclient ports.WhatsAppClient, responder ports.AgentResponder, config config.Config) (*RuleBase, error) {
	if responder == nil {
		return nil, errors.New("agent responder is required")
	}

	return &RuleBase{
		waclient:  waclient,
		responder: responder,
		config:    config,
	}, nil
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

func (n *RuleBase) waitingMessage() string {
	strips := "_"
	base_message := "🤖 Processing to AI Agent, Please Wait.. "
	base_message += "Thingking Process: https://smith.langchain.com/ ."
	base_message += fmt.Sprintf(" (LangSmith > Tracing > %s)", n.config.LangSmith.ProjectName)
	return strips + base_message + strips
}

func (h *RuleBase) isCommand(text string) bool {
	return strings.HasPrefix(text, "/")
}

func (h *RuleBase) CommandHandler(v *events.Message, text string) bool {
	text = strings.TrimSpace(text)
	commands := registeredCommands()
	args := CommandArgs{
		Text: text,
		Evt:  v,
	}
	for _, cmd := range commands.All() {
		if strings.HasPrefix(text, cmd.Command) {
			if cmd.Function == nil {
				log.Printf("command '%s' has no handler", cmd.Command)
				return false
			}
			cmd.Function(h, args)
			return true
		}
	}

	return false
}

func (h *RuleBase) ChatHandler(v *events.Message, text string) {
	if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), h.waitingMessage()); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}

	pkg.Marker(pkg.IoColorYellow, v.Info.Sender.User)
	replyText, err := h.responder.Reply(context.Background(), text, v.Info.Sender)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), fmt.Sprintf("🤖_Sorry, I encountered an error while processing your request._\nError Details: %s", err.Error())); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), replyText); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}

	log.Printf("reply sent to %s", v.Info.Chat.String())
}

func (h *RuleBase) HandleMessageEvent(v *events.Message) {
	if !handleMessageEventGuard(h) {
		return
	}
	if contact_info := h.procContactMessage(v); contact_info != nil {
		h.ChatHandler(v, fmt.Sprintf("Received contact info:\nName: %s\nFull Name: %s\nPhone: %s", contact_info.N, contact_info.FN, contact_info.WAID))
		return
	}
	h.procTextMessage(v)
}
