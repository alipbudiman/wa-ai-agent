package runbot

import (
	"context"
	"fmt"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
)

type Service struct {
	bot ports.BotRunner
}

func NewService(bot ports.BotRunner) *Service {
	return &Service{bot: bot}
}

func (s *Service) Run(ctx context.Context) error {
	if s.bot == nil {
		return fmt.Errorf("bot runner is required")
	}

	return s.bot.Run(ctx)
}
