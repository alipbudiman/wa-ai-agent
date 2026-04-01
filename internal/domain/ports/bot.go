package ports

import "context"

type BotRunner interface {
	Run(ctx context.Context) error
}

type AgentResponder interface {
	Reply(ctx context.Context, prompt string) (string, error)
	SendText(ctx context.Context, to string, text string) error
}
