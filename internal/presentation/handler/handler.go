package eventshandler

import (
	"log"

	"go.mau.fi/whatsmeow/types/events"
)

type MessageHandler interface {
	HandleMessageEvent(*events.Message)
}

func HandleEvent(messageHandler MessageHandler) func(evt any) {
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
			// ignore other event types
		}
	}
}
