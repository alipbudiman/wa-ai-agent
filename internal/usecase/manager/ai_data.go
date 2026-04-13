package manager

import (
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
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
