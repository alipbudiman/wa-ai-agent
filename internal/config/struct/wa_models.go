package cfg_struct

import (
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type WhatsAppConfigDB struct {
	ConfigName      string `bson:"config_name"`
	RequireFullSync bool   `bson:"require_full_sync"`
}

type LangSmithConfigDB struct {
	ConfigName  string `bson:"config_name"`
	APIKey      string `bson:"api_key"`
	ProjectName string `bson:"project_name"`
}

type WhatsAppConfig struct {
	Dialect         string
	SessionDSN      string
	DBLogLevel      string
	ClientLogLevel  string
	RequireFullSync func() bool
}

type LangSmithConfig struct {
	APIKey      func() string
	ProjectName func() string
}

func (LangSmithConfig) GetLangSmithConfig() LangSmithConfig {
	var config LangSmithConfigDB
	filter := bson.M{"config_name": "langsmith"}
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		filter,
		&config,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			config_data := LangSmithConfigDB{
				ConfigName:  "langsmith",
				APIKey:      "",
				ProjectName: "wa_bot_ai_trace",
			}
			res, err := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				config_data,
			)
			if err != nil {
				return LangSmithConfig{
					APIKey:      func() string { return "" },
					ProjectName: func() string { return "wa_bot_ai_trace" },
				}
			}
			if res.InsertedID == nil {
				return LangSmithConfig{
					APIKey:      func() string { return "" },
					ProjectName: func() string { return "wa_bot_ai_trace" },
				}
			}
			return LangSmithConfig{
				APIKey:      func() string { return "" },
				ProjectName: func() string { return "wa_bot_ai_trace" },
			}
		}
	}
	return LangSmithConfig{
		APIKey:      func() string { return config.APIKey },
		ProjectName: func() string { return config.ProjectName },
	}

}

func (WhatsAppConfig) GetRequireFullSyncConfig() bool {
	var config WhatsAppConfigDB
	filter := bson.M{"config_name": "whatsapp"}
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		filter,
		&config,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			config_data := WhatsAppConfigDB{
				ConfigName:      "whatsapp",
				RequireFullSync: false,
			}
			res, err := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				config_data,
			)
			if err != nil {
				return false
			}
			if res.InsertedID == nil {
				return false
			}
			return false
		}
	}
	return config.RequireFullSync
}
