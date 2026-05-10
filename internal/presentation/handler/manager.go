package handler

import (
	"context"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	rulebase "github.com/alipbudiman/wa-ai-agent.git/internal/usecase/agent"
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

// TODO: will implemented for GUI
func GetAIDataStatus(ctx context.Context, chatCompleter ports.ChatCompleter) (*ports.AIDataStatus, error) {
	return chatCompleter.GetAIDataStatus(ctx)
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

func GetToolsBuilderList(manager ports.Manager, agentType string) ([]ports.ToolBuilderData, error) {
	return manager.GetToolsBuilderList(agentType)
}

func AddToolBuilder(manager ports.Manager, tool ports.ToolBuilderData) error {
	return manager.AddToolBuilder(tool)
}

func UpdateToolBuilder(manager ports.Manager, agentType string, originalName string, tool ports.ToolBuilderData) error {
	return manager.UpdateToolBuilder(agentType, originalName, tool)
}

func DeleteToolBuilder(manager ports.Manager, agentType string, name string) error {
	return manager.DeleteToolBuilder(agentType, name)
}

func GetAllCommands(manager ports.Manager) ([]string, error) {
	register := rulebase.RegisteredCommands()
	var commandStr []string
	for _, cmd := range register.All() {
		commandStr = append(commandStr, cmd.Command)
	}
	return commandStr, nil
}

// TODO: GUI - add this function into get provider via GUI
func GetSelectedAIProvider(manager ports.Manager) (string, error) {
	return manager.GetSelectedAIProvider()
}

// TODO: GUI - add this function into set provider via GUI
func SetSelectedAIProvider(manager ports.Manager, providerName string) error {
	return manager.SetSelectedAIProvider(providerName)
}

func StoreAIMessageToInbox(manager ports.Manager, message string, senderNumber string) error {
	return manager.StoreAIMessageToInbox(message, senderNumber)
}

func InjectAIConversation(chatCompleter ports.ChatCompleter, conversation string, senderNumber string) (bool, error) {
	return chatCompleter.InjectAIConversation(context.Background(), conversation, senderNumber)
}
