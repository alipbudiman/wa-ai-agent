package cfg_struct

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	OLLAMA_CONFIG_NAME     = "ollama"
	OPENROUTER_CONFIG_NAME = "openrouter"
	NVIDIA_CONFIG_NAME     = "nvidia"
	ANTHROPIC_CONFIG_NAME  = "anthropic"
	GOOGLE_CONFIG_NAME     = "google"
	MINIMAX_CONFIG_NAME    = "minimax"
	OPEN_AI_CONFIG_NAME    = "openai"
)

type OllamaConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	BaseURL    string `bson:"base_url" json:"base_url"`
	Model      string `bson:"model" json:"model"`
}

type OpenRouterConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
	Model      string `bson:"model" json:"model"`
}

type NvidiaConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
	Model      string `bson:"model" json:"model"`
}

type AnthropicConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	BaseURL    string `bson:"base_url" json:"base_url"`
	APIKey     string `bson:"api_key" json:"api_key"`
	Model      string `bson:"model" json:"model"`
	AuthToken  string `bson:"auth_token" json:"auth_token"`
}

type GoogleConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
	ImgModel   string `bson:"img_model" json:"img_model"`
	Model      string `bson:"model" json:"model"`
}

type MinimaxConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
	GroupID    string `bson:"group_id" json:"group_id"`
	ImgModel   string `bson:"img_model" json:"img_model"`
	Model      string `bson:"model" json:"model"`
}

type OpenAIConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
	Model      string `bson:"model" json:"model"`
	ImgModel   string `bson:"img_model" json:"img_model"`
	ImgQuality string `bson:"img_quality" json:"img_quality"`
}

func rawBSONMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case bson.M:
		return map[string]any(typed), true
	case *bson.M:
		if typed == nil {
			return nil, false
		}
		return map[string]any(*typed), true
	case map[string]any:
		return typed, true
	case *map[string]any:
		if typed == nil {
			return nil, false
		}
		return *typed, true
	default:
		return nil, false
	}
}

func KeyUpdate(formDB any, formDefault any) *bson.M {
	dbFields, ok := rawBSONMap(formDB)
	if !ok {
		return nil
	}

	defaultValue := reflect.Indirect(reflect.ValueOf(formDefault))
	if !defaultValue.IsValid() {
		return nil
	}

	if defaultValue.Kind() != reflect.Struct {
		return nil
	}

	updateSet := bson.M{}
	defaultType := defaultValue.Type()

	for i := 0; i < defaultValue.NumField(); i++ {
		field := defaultType.Field(i)
		tag := field.Tag.Get("bson")
		if tag == "" || tag == "-" {
			continue
		}

		// Handle kalau ada opsi seperti `bson:"name,omitempty"`
		key := strings.Split(tag, ",")[0]
		if key == "" || key == "-" || key == "config_name" {
			continue
		}

		if _, exists := dbFields[key]; exists {
			continue
		}

		defaultField := defaultValue.Field(i)
		if !defaultField.CanInterface() {
			continue
		}

		switch defaultField.Kind() {
		case reflect.Func, reflect.Chan, reflect.UnsafePointer:
			continue
		}

		updateSet[key] = defaultField.Interface()
	}

	if len(updateSet) == 0 {
		return nil
	}

	update := bson.M{"$set": updateSet}
	return &update
}

func decodeRawConfig[T any](rawConfig bson.M, fallback T) T {
	data, err := bson.Marshal(rawConfig)
	if err != nil {
		return fallback
	}

	var config T
	if err := bson.Unmarshal(data, &config); err != nil {
		return fallback
	}

	return config
}

func FindConfigWithDefaults[T any](filter bson.M, defaultConfig T, label string) T {
	var rawConfig bson.M
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		filter,
		&rawConfig,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			res, insertErr := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				defaultConfig,
			)
			if insertErr != nil || res.InsertedID == nil {
				return defaultConfig
			}
		}
		return defaultConfig
	}

	update := KeyUpdate(rawConfig, defaultConfig)
	if update != nil {
		_, err = databases.MongoManager.UpdateOne(
			Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
			Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
			BOT_CONFIG_COLLECTION_NAME,
			filter,
			*update,
		)
		if err != nil {
			fmt.Printf("Error updating %s config with default values: %v\n", label, err)
		} else if updateSet, ok := (*update)["$set"].(bson.M); ok {
			for key, value := range updateSet {
				rawConfig[key] = value
			}
		}
	}

	return decodeRawConfig(rawConfig, defaultConfig)
}

func GetOllamaConfig() OllamaConfig {
	default_config := OllamaConfig{
		ConfigName: OLLAMA_CONFIG_NAME,
		BaseURL:    Env("OLLAMA_BASE_URL", "http://localhost:11434"),
		Model:      Env("OLLAMA_MODEL", "minimax-m2.7:cloud"),
	}

	filter := bson.M{"config_name": OLLAMA_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "Ollama")
}

func UpdateOllamaConfig(config OllamaConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.BaseURL != "" {
		data_config["$set"].(bson.M)["base_url"] = config.BaseURL
	}
	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": OLLAMA_CONFIG_NAME},
		data_config,
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil

}

func GetOpenRouterConfig() OpenRouterConfig {
	default_config := OpenRouterConfig{
		ConfigName: OPENROUTER_CONFIG_NAME,
		APIKey:     Env("OPENROUTER_API_KEY", ""),
		Model:      Env("OPENROUTER_MODEL", "gpt-3.5-turbo"),
	}

	filter := bson.M{"config_name": OPENROUTER_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "OpenRouter")
}

func UpdateOpenRouterConfig(config OpenRouterConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		data_config["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		data_config["$set"].(bson.M)["api_key"] = config.APIKey
	}
	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": OPENROUTER_CONFIG_NAME},
		data_config,
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil

}

func GetNvidiaConfig() NvidiaConfig {
	default_config := NvidiaConfig{
		ConfigName: "nvidia",
		APIKey:     Env("NVIDIA_API_KEY", ""),
		Model:      Env("NVIDIA_MODEL", "nvidia/gpt-3.5-turbo"),
	}

	filter := bson.M{"config_name": NVIDIA_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "Nvidia")
}

func UpdateNvidiaConfig(config NvidiaConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		data_config["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		data_config["$set"].(bson.M)["api_key"] = config.APIKey
	}

	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": NVIDIA_CONFIG_NAME},
		data_config,
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil

}

func GetAnthropicConfig() AnthropicConfig {
	default_config := AnthropicConfig{
		ConfigName: "anthropic",
		BaseURL:    Env("ANTHROPIC_BASE_URL", "http://localhost:11434"),
		APIKey:     Env("ANTHROPIC_API_KEY", ""),
		Model:      Env("ANTHROPIC_MODEL", "claude-2"),
		AuthToken:  Env("ANTHROPIC_AUTH_TOKEN", ""),
	}

	filter := bson.M{"config_name": ANTHROPIC_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "Anthropic")
}

func UpdateAnthropicConfig(config AnthropicConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.BaseURL != "" {
		data_config["$set"].(bson.M)["base_url"] = config.BaseURL
	}
	if config.APIKey == "-DELETE-" {
		data_config["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		data_config["$set"].(bson.M)["api_key"] = config.APIKey
	}
	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}
	if config.AuthToken == "-DELETE-" {
		data_config["$set"].(bson.M)["auth_token"] = ""
	} else if config.AuthToken != "" {
		data_config["$set"].(bson.M)["auth_token"] = config.AuthToken
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": ANTHROPIC_CONFIG_NAME},
		data_config,
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func GetGoogleConfig() GoogleConfig {
	default_config := GoogleConfig{
		ConfigName: GOOGLE_CONFIG_NAME,
		APIKey:     Env("GOOGLE_API_KEY", ""),
		ImgModel:   Env("GOOGLE_IMG_MODEL", "gemini-2.5-flash-lite"),
		Model:      Env("GOOGLE_MODEL", "gemini-2.5-flash-lite"),
	}

	filter := bson.M{"config_name": GOOGLE_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "Google")
}

func UpdateGoogleConfig(config GoogleConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		data_config["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		data_config["$set"].(bson.M)["api_key"] = config.APIKey
	}
	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}
	if config.ImgModel != "" {
		data_config["$set"].(bson.M)["img_model"] = config.ImgModel
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": GOOGLE_CONFIG_NAME},
		data_config,
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func GetMinimaxConfig() MinimaxConfig {
	default_config := MinimaxConfig{
		ConfigName: MINIMAX_CONFIG_NAME,
		APIKey:     Env("MINIMAX_API_KEY", ""),
		GroupID:    Env("MINIMAX_GROUP_ID", ""),
		ImgModel:   Env("MINIMAX_IMG_MODEL", "image-01"),
		Model:      Env("MINIMAX_MODEL", "MiniMax-M2.7"),
	}

	filter := bson.M{"config_name": MINIMAX_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "Minimax")
}

func UpdateMinimaxConfig(config MinimaxConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		data_config["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		data_config["$set"].(bson.M)["api_key"] = config.APIKey
	}
	if config.GroupID == "-DELETE-" {
		data_config["$set"].(bson.M)["group_id"] = ""
	} else if config.GroupID != "" {
		data_config["$set"].(bson.M)["group_id"] = config.GroupID
	}
	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}
	if config.ImgModel != "" {
		data_config["$set"].(bson.M)["img_model"] = config.ImgModel
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": MINIMAX_CONFIG_NAME},
		data_config,
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func GetOpenAIConfig() OpenAIConfig {
	default_config := OpenAIConfig{
		ConfigName: OPEN_AI_CONFIG_NAME,
		APIKey:     Env("OPENAI_API_KEY", ""),
		Model:      Env("OPENAI_MODEL", "gpt-5"),
		ImgModel:   Env("OPENAI_IMG_MODEL", "gpt-image-2-2026-04-21"),
		ImgQuality: Env("OPENAI_IMG_QUALITY", "medium"),
	}

	filter := bson.M{"config_name": OPEN_AI_CONFIG_NAME}
	return FindConfigWithDefaults(filter, default_config, "OpenAI")
}

func UpdateOpenAIConfig(config OpenAIConfig) error {

	data_config := bson.M{
		"$set": bson.M{},
	}

	if config.APIKey == "-DELETE-" {
		data_config["$set"].(bson.M)["api_key"] = ""
	} else if config.APIKey != "" {
		data_config["$set"].(bson.M)["api_key"] = config.APIKey
	}
	if config.Model != "" {
		data_config["$set"].(bson.M)["model"] = config.Model
	}

	if config.ImgModel != "" {
		data_config["$set"].(bson.M)["img_model"] = config.ImgModel
	}

	if config.ImgQuality != "" {
		data_config["$set"].(bson.M)["img_quality"] = config.ImgQuality
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": OPEN_AI_CONFIG_NAME},
		data_config,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}
	return nil

}
