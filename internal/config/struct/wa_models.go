package cfg_struct

import (
	"fmt"

	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	TAVILY_CONFIG_NAME    string = "tavily"
	LANGSMITH_CONFIG_NAME string = "langsmith"
)

type WhatsAppConfigDB struct {
	ConfigName      string `bson:"config_name"`
	RequireFullSync bool   `bson:"require_full_sync"`
}

type TavilyConfigDB struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
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

func GetTavilyConfig() TavilyConfigDB {
	defaultConfig := TavilyConfigDB{
		ConfigName: TAVILY_CONFIG_NAME,
		APIKey:     Env("TAVILY_API_KEY", ""),
	}

	filter := bson.M{"config_name": TAVILY_CONFIG_NAME}
	return FindConfigWithDefaults(filter, defaultConfig, "Tavily")
}

func UpdateTavilyConfig(config TavilyConfigDB) error {
	GetTavilyConfig()

	dataConfig := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		dataConfig["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		dataConfig["$set"].(bson.M)["api_key"] = config.APIKey
	}

	if len(dataConfig["$set"].(bson.M)) == 0 {
		return fmt.Errorf("no fields to update")
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": TAVILY_CONFIG_NAME},
		dataConfig,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func (LangSmithConfig) GetLangSmithConfig() LangSmithConfig {
	var config LangSmithConfigDB
	filter := bson.M{"config_name": LANGSMITH_CONFIG_NAME}
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
				ConfigName:  LANGSMITH_CONFIG_NAME,
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

func UpdateLangSmithConfig(config LangSmithConfigDB) error {
	LangSmithConfig{}.GetLangSmithConfig()

	dataConfig := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		dataConfig["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		dataConfig["$set"].(bson.M)["api_key"] = config.APIKey
	}
	if config.ProjectName != "" {
		dataConfig["$set"].(bson.M)["project_name"] = config.ProjectName
	}

	if len(dataConfig["$set"].(bson.M)) == 0 {
		return fmt.Errorf("no fields to update")
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": LANGSMITH_CONFIG_NAME},
		dataConfig,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
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
