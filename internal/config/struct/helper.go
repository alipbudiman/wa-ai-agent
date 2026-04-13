package cfg_struct

import (
	"log"
	"os"
)

const (
	BOT_CONFIG_COLLECTION_NAME    string = "manager_bot_config"
	ACCESS_COLLECTION_NAME        string = "manager_access"
	TEMPLATE_MSG_COLLECTION_NAME  string = "tmp_messages"
	LOG_SYSTEM_COLLECTION_NAME    string = "log_system"
	INBOX_COLLECTION_NAME         string = "inbox"
	INPUT_HISTORY_COLLECTION_NAME string = "input_history"
)

func BoolEnv(key string, defaultVal bool) bool {
	valStr := Env(key, "")
	if valStr == "" {
		return defaultVal
	}
	return valStr == "true" || valStr == "1"
}

func Env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func GinModEnv(key string, fallback GinMod) GinMod {
	if value := os.Getenv(key); value != "" {
		return GinMod(value)
	}

	log.Printf("Environment variable %s not set, using default: %s", key, fallback)
	return fallback
}
