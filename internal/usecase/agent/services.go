package rulebase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	ai_providers_api "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	errNilMessageEvent  = errors.New("message event is nil")
	errIgnoreOwnMessage = errors.New("ignore own message")
	errEmptyTextMessage = errors.New("message does not contain text content")
	waitingMessage      = "⏳ _Processing your request..._"
)

type Agents struct {
	waclient  ports.WhatsAppClient
	responder ports.AgentResponder
	config    config.Config
	chatLocks sync.Map
}

func NewAgents(waclient ports.WhatsAppClient, responder ports.AgentResponder, config config.Config) (*Agents, error) {
	if responder == nil {
		return nil, errors.New("agent responder is required")
	}

	return &Agents{
		waclient:  waclient,
		responder: responder,
		config:    config,
	}, nil
}

func (h *Agents) withChatLock(chatID string, fn func()) {
	if fn == nil {
		return
	}

	if chatID == "" {
		fn()
		return
	}

	value, _ := h.chatLocks.LoadOrStore(chatID, &sync.Mutex{})
	chatLock, ok := value.(*sync.Mutex)
	if !ok || chatLock == nil {
		fn()
		return
	}

	chatLock.Lock()
	defer chatLock.Unlock()
	fn()
}

func (r *Agents) handleMessage(v *events.Message) (string, error) {
	if v == nil || v.Message == nil || v.Message.GetConversation() == "" && v.Message.GetExtendedTextMessage() == nil {
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

func (n *Agents) langSmithMessage() string {
	strips := "_"
	base_message := "See Process on https://smith.langchain.com/ ."
	base_message += fmt.Sprintf(" (LangSmith > Tracing > %s)", n.config.LangSmith.ProjectName)
	return strips + base_message + strips
}

func (h *Agents) isCommand(text string) bool {
	return strings.HasPrefix(text, "/")
}

func (h *Agents) CommandHandler(v *events.Message, text string) bool {
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

func (h *Agents) ChatHandler(v *events.Message, text string) {
	replyStatus, err := h.responder.Reply(context.Background(), text, v.Info.Sender)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), fmt.Sprintf("🤖_Sorry, I encountered an error while processing your request._\nError Details: %s", err.Error())); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendQuoteMessage(context.Background(), v.Info.Chat.String(), h.langSmithMessage(), waitingMessage); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}

	log.Printf("success process message to ai from %s", v.Info.Chat.String())
}

func (h *Agents) HandleMessageEvent(v *events.Message) {
	if v == nil {
		return
	}

	h.withChatLock(v.Info.Chat.String(), func() {
		if v.Info.Chat.Server == types.BroadcastServer {
			return
		}
		if !h.handleMessageEventGuard() {
			return
		}
		if contact_info := h.procContactMessage(v); contact_info != nil {
			h.ChatHandler(v, fmt.Sprintf("Received contact info:\nName: %s\nFull Name: %s\nPhone: %s", contact_info.N, contact_info.FN, contact_info.WAID))
			return
		}
		h.procTextMessage(v)
	})
}
