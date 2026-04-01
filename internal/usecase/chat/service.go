package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	openai "github.com/sashabaranov/go-openai"
	"go.mau.fi/whatsmeow/types"
)

var (
	errEmptyPrompt        = errors.New("prompt cannot be empty")
	errEmptyChatCompleter = errors.New("chat completer is not configured")
)

type Service struct {
	waClient      ports.WhatsAppClient
	chatCompleter ports.ChatCompleter
	model         string
}

func NewService(waClient ports.WhatsAppClient, chatCompleter ports.ChatCompleter, model string) *Service {
	return &Service{
		waClient:      waClient,
		chatCompleter: chatCompleter,
		model:         strings.TrimSpace(model),
	}
}

func (s *Service) Reply(ctx context.Context, prompt string, jid types.JID) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errEmptyPrompt
	}

	if s.chatCompleter == nil {
		return "", errEmptyChatCompleter
	}

	reply, err := s.chatCompleter.ChatCompletion(ctx, s.model, []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}, jid.User)
	if err != nil {
		return "", fmt.Errorf("generate ai reply: %w", err)
	}

	return strings.TrimSpace(reply), nil
}
