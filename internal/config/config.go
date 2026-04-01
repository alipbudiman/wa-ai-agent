package config

import (
	"log"
	"os"
)

type Config struct {
	// WhatsApp configuration for WhatsApp client and AI Agent
	WhatsApp WhatsAppConfig
	AIAgent  AIAgentConfig
	ModelRouting ModelRouting
	LangSmith LangSmithConfig

	// GinServer configuration for Gin HTTP server
	GinServer GinServerConfig
}

func Load() Config {
	return Config{
		// WhatsApp Bot & AI Agent configuration
		WhatsApp: WhatsAppConfig{
			Dialect:        env("WA_SQL_DIALECT", "sqlite"),
			SessionDSN:     env("WA_SESSION_DSN", "file:wa_session.db?_pragma=foreign_keys(1)"),
			DBLogLevel:     env("WA_DB_LOG_LEVEL", "INFO"),
			ClientLogLevel: env("WA_CLIENT_LOG_LEVEL", "INFO"),
		},
		AIAgent: AIAgentConfig{
			OpenRouter: OpenRouterConfig{
				APIKey: env("OPENROUTER_API_KEY", ""),
				Model:  env("OPENROUTER_MODEL", "qwen/qwen3.6-plus-preview:free"),
				URL:    env("OPENROUTER_URL", "https://openrouter.ai/api/v1"),
			},
		},
		ModelRouting: ModelRouting{
			ToolsSelection: AIAgentConfig{
				OpenRouter: OpenRouterConfig{
					APIKey: env("OPENROUTER_API_KEY", ""),
					Model:  env("OPENROUTER_MODEL", "qwen/qwen3.6-plus-preview:free"),
					URL:    env("OPENROUTER_URL", "https://openrouter.ai/api/v1"),
				},
			},
		},
		LangSmith: LangSmithConfig{
			APIKey:     env("LANGSMITH_API_KEY", ""),
			ProjectName: env("LANGSMITH_PROJECT_NAME", "wa_bot_ai_trace"),
		},

		// Gin Server configuration
		GinServer: GinServerConfig{
			Port: env("GIN_SERVER_PORT", "8080"),
			Host: env("GIN_SERVER_HOST", "localhost"),
			AppMod: ginModEnv("APP_MODE", DebugMode), // options: debug, release, test
		},
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func ginModEnv(key string, fallback GinMod) GinMod {
	if value := os.Getenv(key); value != "" {
		return GinMod(value)
	}

	log.Printf("Environment variable %s not set, using default: %s", key, fallback)
	return fallback
}
