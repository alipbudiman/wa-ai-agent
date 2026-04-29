package rulebase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	ai_providers_api "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"github.com/sashabaranov/go-openai"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	errNilMessageEvent  = errors.New("message event is nil")
	errIgnoreOwnMessage = errors.New("ignore own message")
	errEmptyTextMessage = errors.New("message does not contain text content")
	waitingMessage      = "⏳ _Processing your request..._"
	EmptyJID            = types.JID{}
)

type Agents struct {
	waclient      ports.WhatsAppClient
	responder     ports.AgentResponder
	manager       ports.Manager
	config        config.Config
	chatCompleter ports.ChatCompleter
	chatLocks     sync.Map
}

func NewAgents(waclient ports.WhatsAppClient, responder ports.AgentResponder, manager ports.Manager, chatCompleter ports.ChatCompleter, config config.Config) (*Agents, error) {
	if responder == nil {
		return nil, errors.New("agent responder is required")
	}

	return &Agents{
		waclient:      waclient,
		responder:     responder,
		manager:       manager,
		chatCompleter: chatCompleter,
		config:        config,
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
	base_message += fmt.Sprintf(" (LangSmith > Tracing > %s)", n.config.LangSmith.ProjectName())
	return strips + base_message + strips
}

func (h *Agents) isCommand(text string) bool {
	return strings.HasPrefix(text, "/")
}

func (h *Agents) CommandHandler(v *events.Message, text string) bool {
	text = strings.TrimSpace(text)
	commands := RegisteredCommands()
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
	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				ReasoningContent: h.getReplyMessage(v),
				Role:             h.isFromGroup(v),
				Content:          text,
			},
		},
		false,
		ports.NONE,
		v.Info.Sender,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("🤖_Sorry, I encountered an error while processing your request._\nError Details: %s", err.Error())),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("%s\n%s", waitingMessage, h.langSmithMessage())),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		// not used because quote message only available for phone device
		// if err := h.waclient.SendQuoteMessage(context.Background(), v.Info.Chat.String(), h.langSmithMessage(), waitingMessage); err != nil {
		// 	log.Printf("failed to send reply message: %v", err)
		// 	return
		// }
	}

	log.Printf("success process message to ai from %s", v.Info.Chat.String())
}

func (h *Agents) wrapNoAccessHandler(v *events.Message, content InboxContent) bool {
	type Result struct {
		Value string
		Err   error
	}

	resolvedSender := v.Info.Sender.ToNonAD()
	if !v.Info.SenderAlt.IsEmpty() && v.Info.SenderAlt.Server == types.DefaultUserServer {
		resolvedSender = v.Info.SenderAlt.ToNonAD()
	}

	senderNumber := resolvedSender.User
	if senderNumber == "" {
		log.Printf("sender user is empty, raw sender=%s sender_alt=%s", v.Info.Sender.String(), v.Info.SenderAlt.String())
		return false
	}

	isAllowed, err := h.manager.CheckAccess(senderNumber)
	log.Printf("is allowed %v for user: %s (raw sender=%s sender_alt=%s)", isAllowed, senderNumber, v.Info.Sender.String(), v.Info.SenderAlt.String())
	if err != nil {
		log.Printf("error checking access for number %s: %v", senderNumber, err)
		return false
	}

	if !isAllowed {
		isStroeIboxSuccess := make(chan Result)

		go func() {
			err := h.storeMessageToInbox(v, content, resolvedSender, senderNumber)
			if err != nil {
				log.Printf("error storing message to inbox for user %s: %v", senderNumber, err)
				isStroeIboxSuccess <- Result{Value: "", Err: err}
				return
			}
			isStroeIboxSuccess <- Result{Value: "message stored to inbox successfully", Err: nil}
		}()

		if h.config.App.AIReply.Enable() {

			isReplyAISuccess := make(chan Result)

			go func() {
				err := h.autoReplyAIHandler(v, resolvedSender, content.String())
				if err != nil {
					log.Printf("error in auto-reply handler for user %s: %v", senderNumber, err)
					isReplyAISuccess <- Result{Value: "", Err: err}
					return
				}
				isReplyAISuccess <- Result{Value: "auto-reply process completed", Err: nil}
			}()

			replyResult := <-isReplyAISuccess

			if replyResult.Err != nil {
				h.waclient.SendText(context.Background(), v.Info.Chat.String(), &waE2E.Message{
					Conversation: proto.String(fmt.Sprintf("🤖_Sorry, I encountered an error while processing your request._\nError Details: %s", replyResult.Err.Error())),
				})
				log.Printf("failed to process auto-reply for user %s: %v", senderNumber, replyResult.Err)
			} else {
				log.Printf("successfully processed auto-reply for user %s", senderNumber)
			}
		}

		storeResult := <-isStroeIboxSuccess

		if storeResult.Err != nil {
			log.Printf("failed to store message to inbox for user %s: %v", senderNumber, storeResult.Err)
		} else {
			log.Printf("successfully stored message to inbox for user %s", senderNumber)
		}

		return true
	}

	return false
}

func (h *Agents) autoReplyAIHandler(v *events.Message, resolvedSender types.JID, text string) error {

	if v.Info.Chat.Server == types.GroupServer {
		if !h.config.App.AIGroupInterraction.Enable() {
			return nil
		}
	}
	pkg.Marker(pkg.IoColorCyan, "server:", v.Info.Chat.Server, "config interract:", h.config.App.AIGroupInterraction.Enable())

	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				ReasoningContent: h.getReplyMessage(v),
				Role:             h.isFromGroup(v),
				Content:          text,
			},
		},
		false,
		ports.NONE,
		resolvedSender,
		RESEARCH_AGENT_ROUTE,
	)
	if err != nil {
		log.Printf("error in auto-reply handler: %v", err)
		return err
	}

	if replyStatus == ai_providers_api.SUCCESS {
		log.Printf("success process message to ai from %s", v.Info.Chat.String())
	}
	return nil
}

func (h *Agents) storeMessageToInbox(v *events.Message, content InboxContent, resolvedSender types.JID, senderNumber string) error {
	if v.Info.Chat.Server == types.GroupServer {
		return nil
	}

	contact_info, err := h.waclient.GetContact(context.Background(), resolvedSender.String())
	if err != nil {
		log.Printf("error getting contact info for sender %s: %v", resolvedSender.String(), err)
		return err
	}

	if !contact_info.Found {
		return fmt.Errorf("contact info not found for sender %s", resolvedSender.String())
	}

	address, err := h.manager.GetInboxAddress()
	if err != nil {
		log.Printf("error getting inbox address: %v", err)
		return err
	}

	if address != EmptyJID {
		contentMsg := "*Message Received Notification*"
		name := contact_info.PushName
		if name == "" {
			name = contact_info.FullName
		}
		if name != "" {
			contentMsg += fmt.Sprintf("\nFrom: %s", name)
		}
		contentMsg += fmt.Sprintf("\nNumber: %s (%s)", senderNumber, contact_info.FullName)
		contentMsg += fmt.Sprintf("\nMessage:\n%s", content.String())

		if err := h.waclient.SendText(context.Background(), address.String(), &waE2E.Message{
			Conversation: proto.String(contentMsg),
		}); err != nil {
			log.Printf("error sending text message to inbox address: %v", err)
		}

		if content.IsContact() {
			time.Sleep(2 * time.Second)

			n := content.ContactInfo.N
			if n == "" {
				n = content.ContactInfo.FN
			}
			if err := h.waclient.SendContact(context.Background(), address.String(), ports.ContactCard{
				FullName: n,
				Number:   content.ContactInfo.WAID,
			}, &waE2E.ContextInfo{
				IsForwarded:     proto.Bool(true),
				ForwardingScore: proto.Uint32(1),
			},
			); err != nil {
				log.Printf("error sending contact message to inbox address: %v", err)
			}
		}
	}

	gen_msg_id := pkg.GenerateUID()
	err = h.manager.StoreInboxMessage(ports.InboxMessage{
		JID:        resolvedSender,
		MsgID:      gen_msg_id,
		FullName:   contact_info.FullName,
		PushName:   contact_info.PushName,
		Number:     senderNumber,
		Message:    content.String(),
		FromMe:     v.Info.IsFromMe,
		ReceivedAt: time.Now().Unix(),
		Read:       false,
	})
	if err != nil {
		time.Sleep(2 * time.Second)
		if err := h.waclient.SendText(context.Background(), address.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("🤖_Sorry, I encountered an error while storing your message to inbox._\nError Details: %s", err.Error())),
		}); err != nil {
			log.Printf("error sending text message to user: %v", err)
		}
	}
	return err
}

func (h *Agents) HandleMessageEvent(v *events.Message) {
	if v == nil || v.Message == nil {
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
			if !h.wrapNoAccessHandler(v, InboxContent{
				ContactInfo: *contact_info,
			}) {
				h.ChatHandler(v, contactCardStringFormat(contact_info))
				log.Printf("processed contact message from %s", v.Info.Chat.String())
			}
			return
		}

		if text, err := h.handleMessage(v); err == nil {
			if err != nil {
				log.Printf("error handling message event: %v", err)
			}
			if !h.wrapNoAccessHandler(v, InboxContent{
				Message: text,
			}) {
				h.procTextMessage(v, text)
				log.Printf("processed text message from %s", v.Info.Chat.String())
			}
			return
		}

		log.Printf("received message with unsupported type from %s; ignoring", v.Info.Chat.String())
	})
}
