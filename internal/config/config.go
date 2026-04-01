package config

import (
	"os"
)

type Config struct {
	WhatsApp WhatsAppConfig
	AIAgent  AIAgentConfig
	ModelRouting ModelRouting
}

type WhatsAppConfig struct {
	Dialect        string
	SessionDSN     string
	DBLogLevel     string
	ClientLogLevel string
}

type OpenRouterConfig struct {
	APIKey string
	Model  string
	URL    string
}

type AIAgentConfig struct {
	OpenRouter OpenRouterConfig
}

type ModelRouting struct {
	ToolsSelection AIAgentConfig 
}

func Load() Config {
	return Config{
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

	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
