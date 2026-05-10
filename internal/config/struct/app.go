package cfg_struct

import (
	"fmt"

	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	AI_REPLY_CONFIG_NAME             string = "ai_reply"
	AI_GROUP_INTERACTION_CONFIG_NAME string = "ai_group_interaction"
	AGENT_CONFIG_MODELS_NAME         string = "agent_config"

	agentConfigModelDefault AgentConfigModelsName = "default"
	suffixAgentModel        AgentConfigModelsName = "_model"
)

type AgentConfigModelsName string

type ManagerConfig struct {
	BotConfigCollectionName    string
	AccessCollectionName       string
	TemplateMsgCollectionName  string
	LogSystemCollectionName    string
	InboxCollectionName        string
	InputHistoryCollectionName string
	ToolsBuilderCollectionName string
}

type DynamicManagerConfig struct {
	RandID func() string
}

type AIReplyConfigDB struct {
	ConfigName string `bson:"config_name"`
	Enable     bool   `bson:"enable"`
}

type AIGroupInteractionConfigDB struct {
	ConfigName string `bson:"config_name"`
	Enable     bool   `bson:"enable"`
}

type AgentProvidersAndModelDB struct {
	AgentProvider string `bson:"agent_provider"`
	AgentModel    string `bson:"agent_model"`
}

type AgentConfigModelsDB struct {
	ConfigName     string                   `bson:"config_name"`
	AgentModel     AgentProvidersAndModelDB `bson:"agent_model"`
	AgentCodeModel AgentProvidersAndModelDB `bson:"agent_code_model"`
	ResearchModel  AgentProvidersAndModelDB `bson:"research_model"`
	ReplyModel     AgentProvidersAndModelDB `bson:"reply_model"`
}

type AppConfig struct {
	Manager             ManagerConfig
	Dynamic             DynamicManagerConfig
	AIReply             AIReplyConfig
	AIGroupInterraction AIGroupInteractionConfig
	AgentConfigModels   AgentConfigModels
}

type AIReplyConfig struct {
	Enable func() bool
}

type AIGroupInteractionConfig struct {
	Enable func() bool
}

type AgentProvidersAndModel struct {
	AgentProvider func() string
	AgentModel    func() string
}

type AgentConfigModels struct {
	AgentModel     AgentProvidersAndModel
	AgentCodeModel AgentProvidersAndModel
	ResearchModel  AgentProvidersAndModel
	ReplyModel     AgentProvidersAndModel
}

func (a AppConfig) GetAIReplyConfig() AIReplyConfig {
	var config AIReplyConfigDB
	default_config := AIReplyConfigDB{
		ConfigName: AI_REPLY_CONFIG_NAME,
		Enable:     false,
	}

	filter := bson.M{"config_name": AI_REPLY_CONFIG_NAME}
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
				return AIReplyConfig{}
			}

			if res.InsertedID == nil {
				return AIReplyConfig{}
			}
			return AIReplyConfig{
				Enable: func() bool { return config.Enable },
			}
		}
		return AIReplyConfig{}
	}
	return AIReplyConfig{
		Enable: func() bool { return config.Enable },
	}
}

func UpdateAIReplyConfig(status bool) error {

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AI_REPLY_CONFIG_NAME},
		bson.M{"$set": bson.M{"enable": status}},
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func (a AppConfig) GetAIGroupInteractionConfig() AIGroupInteractionConfig {
	var config AIGroupInteractionConfigDB
	default_config := AIGroupInteractionConfigDB{
		Enable: false,
	}
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AI_GROUP_INTERACTION_CONFIG_NAME},
		&config,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			res, err := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				AIGroupInteractionConfigDB{
					ConfigName: AI_GROUP_INTERACTION_CONFIG_NAME,
					Enable:     default_config.Enable,
				},
			)
			if err != nil {
				return AIGroupInteractionConfig{}
			}
			if res.InsertedID == nil {
				return AIGroupInteractionConfig{}
			}
			return AIGroupInteractionConfig{
				Enable: func() bool { return config.Enable },
			}
		}
		return AIGroupInteractionConfig{}
	}
	return AIGroupInteractionConfig{
		Enable: func() bool { return config.Enable },
	}
}

func UpdateAIGroupInteractionConfig(status bool) error {

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AI_GROUP_INTERACTION_CONFIG_NAME},
		bson.M{"$set": bson.M{"enable": status}},
	)

	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}

func (a AppConfig) GetAgentConfigModels() AgentConfigModels {
	var config AgentConfigModelsDB
	err := databases.MongoManager.FindOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AGENT_CONFIG_MODELS_NAME},
		&config,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			default_config := AgentConfigModelsDB{
				ConfigName:     AGENT_CONFIG_MODELS_NAME,
				AgentModel:     AgentProvidersAndModelDB{AgentProvider: string(agentConfigModelDefault), AgentModel: string(agentConfigModelDefault)},
				AgentCodeModel: AgentProvidersAndModelDB{AgentProvider: string(agentConfigModelDefault), AgentModel: string(agentConfigModelDefault)},
				ResearchModel:  AgentProvidersAndModelDB{AgentProvider: string(agentConfigModelDefault), AgentModel: string(agentConfigModelDefault)},
				ReplyModel:     AgentProvidersAndModelDB{AgentProvider: string(agentConfigModelDefault), AgentModel: string(agentConfigModelDefault)},
			}
			res, err := databases.MongoManager.InsertOne(
				Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
				Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
				BOT_CONFIG_COLLECTION_NAME,
				default_config,
			)
			if err != nil {
				return AgentConfigModels{}
			}
			if res.InsertedID == nil {
				return AgentConfigModels{}
			}
			return AgentConfigModels{
				AgentModel:     AgentProvidersAndModel{AgentProvider: func() string { return default_config.AgentModel.AgentProvider }, AgentModel: func() string { return default_config.AgentModel.AgentModel }},
				AgentCodeModel: AgentProvidersAndModel{AgentProvider: func() string { return default_config.AgentCodeModel.AgentProvider }, AgentModel: func() string { return default_config.AgentCodeModel.AgentModel }},
				ResearchModel:  AgentProvidersAndModel{AgentProvider: func() string { return default_config.ResearchModel.AgentProvider }, AgentModel: func() string { return default_config.ResearchModel.AgentModel }},
				ReplyModel:     AgentProvidersAndModel{AgentProvider: func() string { return default_config.ReplyModel.AgentProvider }, AgentModel: func() string { return default_config.ReplyModel.AgentModel }},
			}
		}
		return AgentConfigModels{}
	}
	return AgentConfigModels{
		AgentModel:     AgentProvidersAndModel{AgentProvider: func() string { return config.AgentModel.AgentProvider }, AgentModel: func() string { return config.AgentModel.AgentModel }},
		AgentCodeModel: AgentProvidersAndModel{AgentProvider: func() string { return config.AgentCodeModel.AgentProvider }, AgentModel: func() string { return config.AgentCodeModel.AgentModel }},
		ResearchModel:  AgentProvidersAndModel{AgentProvider: func() string { return config.ResearchModel.AgentProvider }, AgentModel: func() string { return config.ResearchModel.AgentModel }},
		ReplyModel:     AgentProvidersAndModel{AgentProvider: func() string { return config.ReplyModel.AgentProvider }, AgentModel: func() string { return config.ReplyModel.AgentModel }},
	}
}

func UpdateAgentConfigModels(config AgentConfigModelsDB) error {
	update := bson.M{"$set": bson.M{}}
	if config.AgentModel.AgentProvider != "" || config.AgentModel.AgentModel != "" {
		update["$set"].(bson.M)["agent_model"] = bson.M{
			"agent_provider": config.AgentModel.AgentProvider,
			"agent_model":    config.AgentModel.AgentModel,
		}
	} else if config.AgentCodeModel.AgentProvider != "" || config.AgentCodeModel.AgentModel != "" {
		update["$set"].(bson.M)["agent_code_model"] = bson.M{
			"agent_provider": config.AgentCodeModel.AgentProvider,
			"agent_model":    config.AgentCodeModel.AgentModel,
		}
	} else if config.ResearchModel.AgentProvider != "" || config.ResearchModel.AgentModel != "" {
		update["$set"].(bson.M)["research_model"] = bson.M{
			"agent_provider": config.ResearchModel.AgentProvider,
			"agent_model":    config.ResearchModel.AgentModel,
		}
	} else if config.ReplyModel.AgentProvider != "" || config.ReplyModel.AgentModel != "" {
		update["$set"].(bson.M)["reply_model"] = bson.M{
			"agent_provider": config.ReplyModel.AgentProvider,
			"agent_model":    config.ReplyModel.AgentModel,
		}
	} else {
		return fmt.Errorf("no fields to update")
	}

	res, err := databases.MongoManager.UpdateOne(
		Env("MONGODB_DB_ALIAS", "wa_ai_agent"),
		Env("MONGODB_DB_NAME", "wa_ai_agent_db"),
		BOT_CONFIG_COLLECTION_NAME,
		bson.M{"config_name": AGENT_CONFIG_MODELS_NAME},
		update,
	)
	if err != nil {
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("no document found to update")
	}

	return nil
}
