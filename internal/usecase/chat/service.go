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

func (s *Service) Reply(ctx context.Context, prompt []openai.ChatCompletionMessage, is_system bool, log_level ports.LogLevel, jid types.JID, routing ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	for i := range prompt {
		prompt[i].Content = strings.TrimSpace(prompt[i].Content)
	}
	if len(prompt) == 0 || prompt[0].Content == "" {
		return "", errEmptyPrompt
	}

	if s.chatCompleter == nil {
		return "", errEmptyChatCompleter
	}

	reply, err := s.chatCompleter.ChatCompletion(ctx, prompt, jid.ToNonAD().String(), is_system, log_level, routing...)
	if err != nil {
		return "", fmt.Errorf("generate ai reply: %w", err)
	}

	return strings.TrimSpace(reply), nil
}

func (s *Service) GetLangSmithRuns(ctx context.Context) ([]ports.LangSmithRunReport, error) {
	if s.chatCompleter == nil {
		return nil, errEmptyChatCompleter
	}

	return s.chatCompleter.GetLangSmithRuns(ctx)
}

func (s *Service) GetLangSmithSelectRun(ctx context.Context, runID string) (*ports.LangSmithLogReport, error) {
	if s.chatCompleter == nil {
		return nil, errEmptyChatCompleter
	}

	return s.chatCompleter.GetLangSmithSelectRun(ctx, runID)
}
