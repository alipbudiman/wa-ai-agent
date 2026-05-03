package cfg_struct

import (
	"fmt"

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
	ImgModel   string `bson:"img_model" json:"img_model"`
	Model      string `bson:"model" json:"model"`
}

type OpenAIConfig struct {
	ConfigName string `bson:"config_name" json:"config_name"`
	APIKey     string `bson:"api_key" json:"api_key"`
	Model      string `bson:"model" json:"model"`
}

func GetOllamaConfig() OllamaConfig {
	var config OllamaConfig

	default_config := OllamaConfig{
		ConfigName: OLLAMA_CONFIG_NAME,
		BaseURL:    Env("OLLAMA_BASE_URL", "http://localhost:11434"),
		Model:      Env("OLLAMA_MODEL", "minimax-m2.7:cloud"),
	}

	filter := bson.M{"config_name": OLLAMA_CONFIG_NAME}
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
		return default_config
	}
	return config
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
	var config OpenRouterConfig
	default_config := OpenRouterConfig{
		ConfigName: OPENROUTER_CONFIG_NAME,
		APIKey:     Env("OPENROUTER_API_KEY", ""),
		Model:      Env("OPENROUTER_MODEL", "gpt-3.5-turbo"),
	}

	filter := bson.M{"config_name": OPENROUTER_CONFIG_NAME}
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
		return default_config
	}
	return config
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
	var config NvidiaConfig
	default_config := NvidiaConfig{
		ConfigName: "nvidia",
		APIKey:     Env("NVIDIA_API_KEY", ""),
		Model:      Env("NVIDIA_MODEL", "nvidia/gpt-3.5-turbo"),
	}

	filter := bson.M{"config_name": NVIDIA_CONFIG_NAME}
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
		return default_config
	}
	return config
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
	var config AnthropicConfig
	default_config := AnthropicConfig{
		ConfigName: "anthropic",
		BaseURL:    Env("ANTHROPIC_BASE_URL", "http://localhost:11434"),
		APIKey:     Env("ANTHROPIC_API_KEY", ""),
		Model:      Env("ANTHROPIC_MODEL", "claude-2"),
		AuthToken:  Env("ANTHROPIC_AUTH_TOKEN", ""),
	}

	filter := bson.M{"config_name": ANTHROPIC_CONFIG_NAME}
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
		return default_config
	}
	return config
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
	var config GoogleConfig
	default_config := GoogleConfig{
		ConfigName: GOOGLE_CONFIG_NAME,
		APIKey:     Env("GOOGLE_API_KEY", ""),
		ImgModel:   Env("GOOGLE_IMG_MODEL", "gemini-2.5-flash-lite"),
		Model:      Env("GOOGLE_MODEL", "gemini-2.5-flash-lite"),
	}

	filter := bson.M{"config_name": GOOGLE_CONFIG_NAME}
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
		return default_config
	}
	return config
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
	var config MinimaxConfig
	default_config := MinimaxConfig{
		ConfigName: MINIMAX_CONFIG_NAME,
		APIKey:     Env("MINIMAX_API_KEY", ""),
		ImgModel:   Env("MINIMAX_IMG_MODEL", "image-01"),
		Model:      Env("MINIMAX_MODEL", "MiniMax-M2.7"),
	}

	filter := bson.M{"config_name": MINIMAX_CONFIG_NAME}
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
		return default_config
	}
	return config
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
	var config OpenAIConfig
	default_config := OpenAIConfig{
		ConfigName: "openai",
		APIKey:     Env("OPENAI_API_KEY", ""),
		Model:      Env("OPENAI_MODEL", "gpt-5"),
	}

	filter := bson.M{"config_name": "openai"}
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
		return default_config
	}
	return config
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

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": "openai"},
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
