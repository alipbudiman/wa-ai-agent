package manager

import (
	"net/url"
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mau.fi/whatsmeow/types"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func normalizeInboxMessageID(rawID string) string {
	normalized := strings.TrimSpace(rawID)
	if normalized == "" {
		return ""
	}

	if decoded, err := url.QueryUnescape(normalized); err == nil {
		normalized = strings.TrimSpace(decoded)
	}

	for i := 0; i < 3; i++ {
		candidate := strings.TrimSpace(normalized)
		if len(candidate) < 2 || candidate[0] != '"' || candidate[len(candidate)-1] != '"' {
			break
		}

		normalized = strings.TrimSpace(candidate[1 : len(candidate)-1])
	}

	return strings.TrimSpace(normalized)
}

func buildInboxMessageIDCandidates(rawID string) []string {
	seen := make(map[string]struct{}, 3)
	addCandidate := func(target *[]string, candidate string) {
		clean := strings.TrimSpace(candidate)
		if clean == "" {
			return
		}
		if _, exists := seen[clean]; exists {
			return
		}
		seen[clean] = struct{}{}
		*target = append(*target, clean)
	}

	candidates := make([]string, 0, 3)
	rawTrimmed := strings.TrimSpace(rawID)
	normalized := normalizeInboxMessageID(rawTrimmed)

	addCandidate(&candidates, rawTrimmed)
	addCandidate(&candidates, normalized)
	if normalized != "" {
		addCandidate(&candidates, "\""+normalized+"\"")
	}

	return candidates
}

func buildInboxJIDFilter(jid types.JID) bson.M {
	jidConditions := bson.A{bson.M{"jid": jid}}
	jidString := strings.TrimSpace(jid.String())
	if jidString != "" {
		jidConditions = append(jidConditions, bson.M{"jid": jidString})
	}

	return bson.M{"$or": jidConditions}
}

func (m *Manager) createInboxSetup() error {
	inboxSetup := InboxSetupInfo{
		JID:       types.JID{},
		Parameter: "inbox_setup",
	}
	res, err := databases.MongoManager.InsertOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.BotConfigCollectionName,
		inboxSetup,
	)
	if err != nil {
		return err
	}

	if res.InsertedID == nil {
		return ErrInboxSetupNotFound
	}

	return nil

}

func (m *Manager) GetInboxAddress() (types.JID, error) {
	filter := bson.M{"parameter": "inbox_setup"}
	var inbox_setup_info InboxSetupInfo
	err := databases.MongoManager.FindOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.BotConfigCollectionName,
		filter,
		&inbox_setup_info,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			err = m.createInboxSetup()
			if err != nil {
				return EmptyJID, err
			}
			return EmptyJID, nil
		}
		return EmptyJID, err
	}

	return inbox_setup_info.JID, nil
}

func (m *Manager) SetInboxAddress(number string) error {
	jid, err := types.ParseJID(number)
	if err != nil {
		return err
	}

	filter := bson.M{"parameter": "inbox_setup"}
	update := bson.M{"$set": bson.M{"jid": jid}}
	res, err := databases.MongoManager.UpdateOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.BotConfigCollectionName,
		filter,
		update,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return ErrInboxSetupNotFound
	}

	return nil

}

func (m *Manager) ClearInboxAddress() error {
	filter := bson.M{"parameter": "inbox_setup"}
	update := bson.M{"$set": bson.M{"jid": EmptyJID}}
	res, err := databases.MongoManager.UpdateOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.BotConfigCollectionName,
		filter,
		update,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return m.createInboxSetup()
	}

	return nil
}

func (m *Manager) GetInboxMessages() ([]ports.InboxMessage, error) {
	// Implementation for retrieving inbox messages
	var inboxMessages []ports.InboxMessage
	err := databases.MongoManager.FindAll(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		bson.M{},
		&inboxMessages,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			return []ports.InboxMessage{}, nil
		}
		return nil, err
	}
	return inboxMessages, nil
}

func (m *Manager) DeleteMessage(msg_id string) error {
	msgIDCandidates := buildInboxMessageIDCandidates(msg_id)
	if len(msgIDCandidates) == 0 {
		return ErrInboxMessageNotFound
	}

	filter := bson.M{"msg_id": bson.M{"$in": msgIDCandidates}}
	res, err := databases.MongoManager.DeleteOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		filter,
	)
	if err != nil {
		return err
	}

	if res.DeletedCount == 0 {
		return ErrInboxMessageNotFound
	}
	return nil
}

func (m *Manager) DeleteAllInboxMessagesByJID(jid types.JID) error {
	filter := buildInboxJIDFilter(jid)
	_, err := databases.MongoManager.DeleteMany(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		filter,
	)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) DeleteAllInboxMessages() error {
	_, err := databases.MongoManager.DeleteMany(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		bson.M{},
	)
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) StoreInboxMessage(message ports.InboxMessage) error {
	res, err := databases.MongoManager.InsertOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		message,
	)
	if err != nil {
		return err
	}

	if res.InsertedID == nil {
		return ErrFailedToStoreInboxMessage
	}

	return nil
}

func (m *Manager) GetUnreadInboxMessages(jid types.JID) ([]string, error) {
	var unreadMessages []ports.InboxMessage
	filter := bson.M{
		"$and": bson.A{
			buildInboxJIDFilter(jid),
			bson.M{"$or": bson.A{
				bson.M{"from_me": false},
				bson.M{"from_me": bson.M{"$exists": false}},
			}},
			bson.M{"$or": bson.A{
				bson.M{"read": false},
				bson.M{"read": bson.M{"$exists": false}},
			}},
		},
	}

	err := databases.MongoManager.FindAll(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		filter,
		&unreadMessages,
	)
	if err != nil {
		return nil, err
	}

	var unreadMsgIDs []string
	for _, msg := range unreadMessages {
		normalizedMsgID := normalizeInboxMessageID(msg.MsgID)
		if normalizedMsgID == "" {
			normalizedMsgID = strings.TrimSpace(msg.MsgID)
		}
		if normalizedMsgID == "" {
			continue
		}

		unreadMsgIDs = append(unreadMsgIDs, normalizedMsgID)
	}

	return unreadMsgIDs, nil
}

func (m *Manager) MarkInboxMessageAsRead(msg_id string) error {
	msgIDCandidates := buildInboxMessageIDCandidates(msg_id)
	if len(msgIDCandidates) == 0 {
		return ErrInboxMessageNotFound
	}

	filter := bson.M{"msg_id": bson.M{"$in": msgIDCandidates}}
	update := bson.M{"$set": bson.M{"read": true}}
	res, err := databases.MongoManager.UpdateOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.InboxCollectionName,
		filter,
		update,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return ErrInboxMessageNotFound
	}

	return nil
}

func (m *Manager) GetSelectedAIProvider() (string, error) {
	filter := bson.M{"config_name": "providers"}
	var providerInfo ProviderInfo
	err := databases.MongoManager.FindOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.BotConfigCollectionName,
		filter,
		&providerInfo,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			return "", nil
		}
		return "", err
	}
	return providerInfo.Providers, nil
}
