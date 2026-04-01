package ports

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

type BotRunner interface {
	Run(ctx context.Context) error
}

type AgentResponder interface {
	Reply(ctx context.Context, prompt string, user_id string) (string, error)
	SendText(ctx context.Context, to string, text string) error
}

type ChatCompleter interface {
	ChatCompletion(ctx context.Context, model string, messages []openai.ChatCompletionMessage, user_id string) (string, error)
}
