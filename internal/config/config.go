package config

import (
	"log"
	"os"

	cfg_struct "github.com/alipbudiman/wa-ai-agent.git/internal/config/struct"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
)

type Config struct {
	// WhatsApp configuration for WhatsApp client and AI Agent
	WhatsApp     cfg_struct.WhatsAppConfig
	AIAgent      cfg_struct.AIAgentConfig
	ModelRouting cfg_struct.ModelRouting
	LangSmith    cfg_struct.LangSmithConfig

	// GinServer configuration for Gin HTTP server
	GinServer cfg_struct.GinServerConfig

	// database configuration
	Databases cfg_struct.DatabaseConfig

	// application configuration
	App cfg_struct.AppConfig
}

func Load() Config {
	return Config{
		// WhatsApp Bot & AI Agent configuration
		WhatsApp: cfg_struct.WhatsAppConfig{
			Dialect:        env("WA_SQL_DIALECT", "sqlite"),
			SessionDSN:     env("WA_SESSION_DSN", "file:wa_session.db?_pragma=foreign_keys(1)"),
			DBLogLevel:     env("WA_DB_LOG_LEVEL", "INFO"),
			ClientLogLevel: env("WA_CLIENT_LOG_LEVEL", "INFO"),
			RequireFullSync: boolEnv("WA_REQUIRE_FULL_SYNC", false),
		},
		AIAgent: cfg_struct.AIAgentConfig{
			OpenRouter: cfg_struct.OpenRouterConfig{
				APIKey: env("OPENROUTER_API_KEY", ""),
				Model:  env("OPENROUTER_MODEL", "qwen/qwen3.6-plus-preview:free"),
				URL:    env("OPENROUTER_URL", "https://openrouter.ai/api/v1"),
			},
		},
		ModelRouting: cfg_struct.ModelRouting{
			ToolsSelection: cfg_struct.AIAgentConfig{
				OpenRouter: cfg_struct.OpenRouterConfig{
					APIKey: env("OPENROUTER_API_KEY", ""),
					Model:  env("OPENROUTER_MODEL", "qwen/qwen3.6-plus-preview:free"),
					URL:    env("OPENROUTER_URL", "https://openrouter.ai/api/v1"),
				},
			},
		},
		LangSmith: cfg_struct.LangSmithConfig{
			APIKey:      env("LANGSMITH_API_KEY", ""),
			ProjectName: env("LANGSMITH_PROJECT_NAME", "wa_bot_ai_trace"),
		},

		// Gin Server configuration
		GinServer: cfg_struct.GinServerConfig{
			Port:   env("GIN_SERVER_PORT", "8001"),
			Host:   env("GIN_SERVER_HOST", "localhost"),
			AppMod: ginModEnv("APP_MODE", cfg_struct.DebugMode), // options: debug, release, test
		},

		// database configuration
		Databases: cfg_struct.DatabaseConfig{
			MongoDB: cfg_struct.MongoDBConfig{
				URI:     env("MONGODB_URI", "mongodb://localhost:27017"),
				DBAlias: env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				DBName:  env("MONGODB_DB_NAME", "wa_ai_agent_db"),
			},
		},

		// application configuration
		App: cfg_struct.AppConfig{
			Manager: cfg_struct.ManagerConfig{
				AccessCollectionName:       "manager_access",
				BotConfigCollectionName:    "manager_bot_config",
				TemplateMsgCollectionName:  "tmp_messages",
				LogSystemCollectionName:    "log_system",
				InboxCollectionName:        "inbox",
				InputHistoryCollectionName: "input_history",
			},
			Dynamic: cfg_struct.DynamicManagerConfig{
				RandID: func() string {
					return pkg.GenerateUID() // Replace with actual random ID generation logic
				},
			},
		},
	}
}

func boolEnv(key string, defaultVal bool) bool {
	valStr := env(key, "")
	if valStr == "" {
		return defaultVal
	}
	return valStr == "true" || valStr == "1"
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func ginModEnv(key string, fallback cfg_struct.GinMod) cfg_struct.GinMod {
	if value := os.Getenv(key); value != "" {
		return cfg_struct.GinMod(value)
	}

	log.Printf("Environment variable %s not set, using default: %s", key, fallback)
	return fallback
}
