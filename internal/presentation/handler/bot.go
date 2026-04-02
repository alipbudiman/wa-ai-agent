package handler

import (
	"log"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow/types/events"
)

func EventTrace(messageHandler ports.MessageHandler) func(evt any) {
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
			messageHandler.HandleMessageEvent(v)
		default:
			// skip other events
		}
	}
}
