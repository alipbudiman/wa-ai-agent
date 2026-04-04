package ports

import (
	"context"

	"github.com/sashabaranov/go-openai"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type BotRunner interface {
	Run(ctx context.Context) error
}

type MessageHandler interface {
	HandleMessageEvent(*events.Message)
}

type AgentResponder interface {
	Reply(ctx context.Context, prompt string, jid types.JID) (string, error)
}

type WhatsAppClient interface {
	GetContact(ctx context.Context, number string) (types.ContactInfo, error)
	SendText(ctx context.Context, to string, text string) error
	ParseJID(target string) (types.JID, error)
	SendQuoteMessage(ctx context.Context, to, text, quote_msg string) error
	GetSelfJID(ctx context.Context) (types.JID, error)
}

type ChatCompleter interface {
	ChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessage, user_id string) (string, error)
	GetLangSmithRuns(ctx context.Context) ([]LangSmithRunReport, error)
	GetLangSmithSelectRun(ctx context.Context, runID string) (*LangSmithLogReport, error)
}
