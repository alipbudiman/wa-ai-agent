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
}

func NewService(waClient ports.WhatsAppClient, chatCompleter ports.ChatCompleter) *Service {
	return &Service{
		waClient:      waClient,
		chatCompleter: chatCompleter,
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

	reply, err := s.chatCompleter.ChatCompletion(ctx, []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}, jid.ToNonAD().String())
	if err != nil {
		return "", fmt.Errorf("generate ai reply: %w", err)
	}

	return strings.TrimSpace(reply), nil
}
