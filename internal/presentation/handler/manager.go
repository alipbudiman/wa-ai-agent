package handler

import (
	"context"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow/types"
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

func GetInboxAddress(manager ports.Manager) (types.JID, error) {
	return manager.GetInboxAddress()
}

func SetInboxAddress(manager ports.Manager, number string) error {
	return manager.SetInboxAddress(number)
}

func ClearInboxAddress(manager ports.Manager) error {
	return manager.ClearInboxAddress()
}

func GetInboxMessages(manager ports.Manager) ([]ports.InboxMessage, error) {
	return manager.GetInboxMessages()
}

func DeleteMessage(manager ports.Manager, msg_id string) error {
	return manager.DeleteMessage(msg_id)
}

func DeleteAllInboxMessagesByJID(manager ports.Manager, jid types.JID) error {
	return manager.DeleteAllInboxMessagesByJID(jid)
}

func DeleteAllInboxMessages(manager ports.Manager) error {
	return manager.DeleteAllInboxMessages()
}

func GetUnreadInboxMessages(manager ports.Manager, jid types.JID) ([]string, error) {
	return manager.GetUnreadInboxMessages(jid)
}

func MarkInboxMessageAsRead(manager ports.Manager, msg_id string) error {
	return manager.MarkInboxMessageAsRead(msg_id)
}
