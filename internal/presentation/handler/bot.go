package handler

import (
	"log"
	"runtime/debug"
	"time"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"github.com/gin-gonic/gin"
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
			if v.Info.Timestamp.Unix() < time.Now().Unix()-43200 {
				return
			}

			if messageHandler == nil {
				log.Printf("message handler is not configured; ignoring incoming message")
				return
			}

			// Handle each incoming message in its own goroutine so one slow request
			// (for example waiting for Brain API/LLM) does not block other chats.

			go func(msg *events.Message) {
				defer func() {
					if recovered := recover(); recovered != nil {
						log.Printf("panic while handling message event: %v\n%s", recovered, string(debug.Stack()))
					}
				}()

				if msg == nil || msg.Info.Chat.Server == types.BroadcastServer {
					return
				}

				messageHandler.HandleMessageEvent(msg)
			}(v)
		default:
			if cfg.GinServer.AppMod == gin.DebugMode {
				pkg.StructPrettyPrint(v, pkg.IoColorYellow)
			}
		}
	}
}
