package manager

import (
	"context"
	"log"
	"strings"
	"time"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"go.mau.fi/whatsmeow/types"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (m *Manager) SetSelectedAIProvider(providerName string) error {
	filter := bson.M{"config_name": "providers"}
	update := bson.M{"$set": bson.M{"providers": providerName}}
	res, err := databases.MongoManager.UpdateOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.BotConfigCollectionName,
		filter,
		update,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			providerInfo := ProviderInfo{
				ConfigName: "providers",
				Providers:  providerName,
			}
			res, err := databases.MongoManager.InsertOne(
				m.config.Databases.MongoDB.DBAlias,
				m.config.Databases.MongoDB.DBName,
				m.config.App.Manager.BotConfigCollectionName,
				providerInfo,
			)
			if err != nil {
				return err
			}
			if res.InsertedID == nil {
				return ErrFailedToAddProviders
			}
			return nil
		}
		return err
	}

	if res.MatchedCount == 0 {
		return ErrFailedToAddProviders
	}

	return nil
}

func (m *Manager) StoreAIMessageToInbox(message string, number string) error {

	senderNumber, err := m.waclient.ParseJID(number)
	if err != nil {
		number = strings.Split(number, "@")[0]
		senderNumber = types.NewJID(number, types.DefaultUserServer)
	}

	contact_info, err := m.waclient.GetContact(context.Background(), senderNumber.User)
	if err != nil {
		log.Printf("error getting contact info for sender %s: %v", senderNumber.String(), err)
		return err
	}

	gen_msg_id := pkg.GenerateUID()
	err = m.StoreInboxMessage(ports.InboxMessage{
		JID:        senderNumber,
		MsgID:      gen_msg_id,
		FullName:   contact_info.FullName,
		PushName:   contact_info.PushName,
		Number:     senderNumber.User,
		Message:    message,
		FromMe:     true,
		ReceivedAt: time.Now().Unix(),
		Read:       false,
	})

	if err != nil {
		return err
	}

	return nil
}
