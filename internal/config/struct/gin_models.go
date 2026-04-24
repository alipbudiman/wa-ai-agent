package cfg_struct

import (
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// DebugMode indicates gin mode is debug.
	DebugMode GinMod = "debug"
	// ReleaseMode indicates gin mode is release.
	ReleaseMode GinMod = "release"
	// TestMode indicates gin mode is test.
	TestMode GinMod = "test"
)

type GinMod string

type GinServerConfig struct {
	Host   string `bson:"host" json:"host"`
	Port   string `bson:"port" json:"port"`
	AppMod GinMod `bson:"app_mod" json:"app_mod"`
}

func (GinServerConfig) GetGinConfig() GinServerConfig {
	var config GinServerConfig

	default_config := GinServerConfig{
		Host:   Env("GIN_SERVER_HOST", "localhost"),
		Port:   Env("GIN_SERVER_PORT", "8001"),
		AppMod: GinModEnv("APP_MODE", gin.DebugMode),
	}

	filter := bson.M{"config_name": "gin_server"}
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
				return default_config
			}
			if res.InsertedID == nil {
				return default_config
			}
			return default_config
		}
	}
	return config
}
