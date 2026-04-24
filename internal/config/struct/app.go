package cfg_struct

import (
	"fmt"

	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	AI_REPLY_CONFIG_NAME             = "ai_reply"
	AI_GROUP_INTERACTION_CONFIG_NAME = "ai_group_interaction"
)

type ManagerConfig struct {
	BotConfigCollectionName    string
	AccessCollectionName       string
	TemplateMsgCollectionName  string
	LogSystemCollectionName    string
	InboxCollectionName        string
	InputHistoryCollectionName string
}

type DynamicManagerConfig struct {
	RandID func() string
}

type AIReplyConfigDB struct {
	ConfigName string `bson:"config_name"`
	Enable     bool   `bson:"enable"`
}

type AIGroupInteractionConfigDB struct {
	Enable bool `bson:"enable"`
}

type AppConfig struct {
	Manager             ManagerConfig
	Dynamic             DynamicManagerConfig
	AIReply             AIReplyConfig
	AIGroupInterraction AIGroupInteractionConfig
}

type AIReplyConfig struct {
	Enable func() bool
}

type AIGroupInteractionConfig struct {
	Enable func() bool
}

func (a AppConfig) GetAIReplyConfig() AIReplyConfig {
	var config AIReplyConfigDB
	default_config := AIReplyConfigDB{
		ConfigName: AI_REPLY_CONFIG_NAME,
		Enable:     false,
	}

	filter := bson.M{"config_name": AI_REPLY_CONFIG_NAME}
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		filter,
		&config,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			res, err := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				default_config,
			)
			if err != nil {
				return AIReplyConfig{}
			}

			if res.InsertedID == nil {
				return AIReplyConfig{}
			}
			return AIReplyConfig{
				Enable: func() bool { return config.Enable },
			}
		}
		return AIReplyConfig{}
	}
	return AIReplyConfig{
		Enable: func() bool { return config.Enable },
	}
}

func UpdateAIReplyConfig(status bool) error {

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AI_REPLY_CONFIG_NAME},
		bson.M{"$set": bson.M{"enable": status}},
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func (a AppConfig) GetAIGroupInteractionConfig() AIGroupInteractionConfig {
	var config AIGroupInteractionConfigDB
	default_config := AIGroupInteractionConfigDB{
		Enable: false,
	}
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AI_GROUP_INTERACTION_CONFIG_NAME},
		&config,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			res, err := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				bson.M{"config_name": AI_GROUP_INTERACTION_CONFIG_NAME, "enable": default_config.Enable},
			)
			if err != nil {
				return AIGroupInteractionConfig{}
			}
			if res.InsertedID == nil {
				return AIGroupInteractionConfig{}
			}
			return AIGroupInteractionConfig{
				Enable: func() bool { return config.Enable },
			}
		}
		return AIGroupInteractionConfig{}
	}
	return AIGroupInteractionConfig{
		Enable: func() bool { return config.Enable },
	}
}

func UpdateAIGroupInteractionConfig(status bool) error {

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AI_GROUP_INTERACTION_CONFIG_NAME},
		bson.M{"$set": bson.M{"enable": status}},
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}
