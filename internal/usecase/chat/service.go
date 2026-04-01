package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alipbudiman/wa-ai-agent.git/internal/domain/ports"
	openai "github.com/sashabaranov/go-openai"
)

var (
	errEmptyPrompt        = errors.New("prompt cannot be empty")
	errEmptyChatCompleter = errors.New("chat completer is not configured")
)

type Service struct {
	sender        ports.MessageSender
	chatCompleter ports.ChatCompleter
	model         string
}

func NewService(sender ports.MessageSender, chatCompleter ports.ChatCompleter, model string) *Service {
	return &Service{
		sender:        sender,
		chatCompleter: chatCompleter,
		model:         strings.TrimSpace(model),
	}
}

func (s *Service) SendText(ctx context.Context, to string, text string) error {
	if s.sender == nil {
		return fmt.Errorf("message sender is required")
	}
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("recipient is required")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("message text is required")
	}

	return s.sender.SendText(ctx, to, text)
}

func (s *Service) Reply(ctx context.Context, prompt string, user_id string) (string, error) {
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
	}, user_id)
	if err != nil {
		return "", fmt.Errorf("generate ai reply: %w", err)
	}

	return strings.TrimSpace(reply), nil
}
