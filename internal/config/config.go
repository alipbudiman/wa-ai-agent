package config

import (
	cfg_struct "github.com/alipbudiman/wa-ai-agent.git/internal/config/struct"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
)

type Config struct {
	// GinServer configuration for Gin HTTP server
	GinServer cfg_struct.GinServerConfig

	// database configuration
	Databases cfg_struct.DatabaseConfig

	// WhatsApp configuration for WhatsApp client and AI Agent
	WhatsApp  cfg_struct.WhatsAppConfig
	LangSmith cfg_struct.LangSmithConfig

	// application configuration
	App cfg_struct.AppConfig
}

func Load() Config {
	return Config{
		// WhatsApp Bot & AI Agent configuration
		// Gin Server configuration
		GinServer: cfg_struct.GinServerConfig{
			Port:   cfg_struct.Env("GIN_SERVER_PORT", "8001"),
			Host:   cfg_struct.Env("GIN_SERVER_HOST", "localhost"),
			AppMod: cfg_struct.GinModEnv("APP_MODE", cfg_struct.DebugMode), // options: debug, release, test
		},

		// database configuration
		Databases: cfg_struct.DatabaseConfig{
			MongoDB: cfg_struct.MongoDBConfig{
				URI:     cfg_struct.Env("MONGODB_URI", "mongodb://localhost:27017"),
				DBAlias: cfg_struct.Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				DBName:  cfg_struct.Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
			},
		},

		WhatsApp: cfg_struct.WhatsAppConfig{
			Dialect:         cfg_struct.Env("WA_SQL_DIALECT", "sqlite"),
			SessionDSN:      cfg_struct.Env("WA_SESSION_DSN", "file:wa_session.db?_pragma=foreign_keys(1)"),
			DBLogLevel:      cfg_struct.Env("WA_DB_LOG_LEVEL", "INFO"),
			ClientLogLevel:  cfg_struct.Env("WA_CLIENT_LOG_LEVEL", "INFO"),
			RequireFullSync: cfg_struct.WhatsAppConfig{}.GetRequireFullSyncConfig,
		},

		LangSmith: cfg_struct.LangSmithConfig{
			APIKey: func() string {
				return cfg_struct.LangSmithConfig{}.GetLangSmithConfig().APIKey()
			},
			ProjectName: func() string {
				return cfg_struct.LangSmithConfig{}.GetLangSmithConfig().ProjectName()
			},
		},

		// application configuration
		App: cfg_struct.AppConfig{
			Manager: cfg_struct.ManagerConfig{
				BotConfigCollectionName:    cfg_struct.BOT_CONFIG_COLLECTION_NAME,
				AccessCollectionName:       cfg_struct.ACCESS_COLLECTION_NAME,
				TemplateMsgCollectionName:  cfg_struct.TEMPLATE_MSG_COLLECTION_NAME,
				LogSystemCollectionName:    cfg_struct.LOG_SYSTEM_COLLECTION_NAME,
				InboxCollectionName:        cfg_struct.INBOX_COLLECTION_NAME,
				InputHistoryCollectionName: cfg_struct.INPUT_HISTORY_COLLECTION_NAME,
			},
			AIReply: cfg_struct.AIReplyConfig{
				Enable: func() bool {
					return cfg_struct.AppConfig{}.GetAIReplyConfig().Enable()
				},
			},
			AIGroupInterraction: cfg_struct.AIGroupInteractionConfig{
				Enable: func() bool {
					return cfg_struct.AppConfig{}.GetAIGroupInteractionConfig().Enable()
				},
			},
			AgentConfigModels: cfg_struct.AgentConfigModels{
				AgentModel: cfg_struct.AgentProvidersAndModel{
					AgentProvider: func() string { return cfg_struct.AppConfig{}.GetAgentConfigModels().AgentModel.AgentProvider() },
					AgentModel:    func() string { return cfg_struct.AppConfig{}.GetAgentConfigModels().AgentModel.AgentModel() },
				},
				AgentCodeModel: cfg_struct.AgentProvidersAndModel{
					AgentProvider: func() string { return cfg_struct.AppConfig{}.GetAgentConfigModels().AgentCodeModel.AgentProvider() },
					AgentModel:    func() string { return cfg_struct.AppConfig{}.GetAgentConfigModels().AgentCodeModel.AgentModel() },
				},
				ResearchModel: cfg_struct.AgentProvidersAndModel{
					AgentProvider: func() string { return cfg_struct.AppConfig{}.GetAgentConfigModels().ResearchModel.AgentProvider() },
					AgentModel:    func() string { return cfg_struct.AppConfig{}.GetAgentConfigModels().ResearchModel.AgentModel() },
				},
			},
			Dynamic: cfg_struct.DynamicManagerConfig{
				RandID: func() string {
					return pkg.GenerateUID() // Replace with actual random ID generation logic
				},
			},
		},
	}
}
