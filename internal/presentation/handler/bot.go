package handler

import (
	"log"
	"runtime/debug"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func EventTrace(cfg *config.Config, waclient ports.WhatsAppClient, messageHandler ports.MessageHandler, manager ports.Manager) func(evt any) {
	return func(evt any) {
		switch v := evt.(type) {
		case *events.Connected:
			log.Printf("WhatsApp client connected")
		case *events.Disconnected:
			log.Printf("WhatsApp client disconnected")
		case *events.LoggedOut:
			log.Printf("WhatsApp client logged out")
		case *events.Message:
			if messageHandler == nil {
				log.Printf("message handler is not configured; ignoring incoming message")
				return
			}

			// Handle each incoming message in its own goroutine so one slow request
			// (for example waiting for Brain API/LLM) does not block other chats.
			msg := v

			go func(msg *events.Message) {
				defer func() {
					if recovered := recover(); recovered != nil {
						log.Printf("panic while handling message event: %v\n%s", recovered, string(debug.Stack()))
					}
				}()

				if v.Info.Chat.Server == types.BroadcastServer {
					return
				}

				messageHandler.HandleMessageEvent(msg)
			}(msg)
		default:
			// skip other events
		}
	}
}
