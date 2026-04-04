package handler

import (
	"context"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
)

func GetAccessList(manager ports.Manager) ([]ports.AccessInfo, error) {
	return manager.GetAccessList()
}

func AddAccess(manager ports.Manager, name string, number string, expiredAt int64) error {
	return manager.AddAccess(name, number, expiredAt)
}

func DeleteAccess(manager ports.Manager, number string) error {
	return manager.DeleteAccess(number)
}

func GetLangSmithRuns(ctx context.Context, chatCompleter ports.ChatCompleter) ([]ports.LangSmithRunReport, error) {
	return chatCompleter.GetLangSmithRuns(ctx)
}

func GetLangSmithSelectRun(ctx context.Context, chatCompleter ports.ChatCompleter, runID string) (*ports.LangSmithLogReport, error) {
	return chatCompleter.GetLangSmithSelectRun(ctx, runID)
}
