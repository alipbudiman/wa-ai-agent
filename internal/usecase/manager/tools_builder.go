package manager

import (
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func normalizeToolBuilderText(rawText string) string {
	return strings.TrimSpace(rawText)
}

func normalizeToolBuilderData(tool ports.ToolBuilderData) ports.ToolBuilderData {
	tool.Name = normalizeToolBuilderText(tool.Name)
	tool.Description = normalizeToolBuilderText(tool.Description)
	tool.AgentType = normalizeToolBuilderText(tool.AgentType)
	tool.Information = normalizeToolBuilderText(tool.Information)

	if tool.Code != nil {
		tool.Code.Code = strings.TrimSpace(tool.Code.Code)
		packages := make([]string, 0, len(tool.Code.Package))
		for _, rawPackage := range tool.Code.Package {
			packageName := strings.TrimSpace(rawPackage)
			if packageName != "" {
				packages = append(packages, packageName)
			}
		}
		tool.Code.Package = packages
		if tool.Code.Code == "" {
			tool.Code = nil
		}
	}

	return tool
}

func validateToolBuilderData(tool ports.ToolBuilderData) error {
	if tool.AgentType == "" || tool.Name == "" || tool.Description == "" {
		return ports.ErrInvalidToolBuilderData
	}
	if tool.Information == "" && (tool.Code == nil || tool.Code.Code == "") {
		return ports.ErrInvalidToolBuilderData
	}
	return nil
}

func (m *Manager) toolsBuilderCollectionName() string {
	collectionName := strings.TrimSpace(m.config.App.Manager.ToolsBuilderCollectionName)
	if collectionName == "" {
		return "tools_builder"
	}
	return collectionName
}

func (m *Manager) GetToolsBuilderList(agentType string) ([]ports.ToolBuilderData, error) {
	agentType = normalizeToolBuilderText(agentType)
	if agentType == "" {
		return []ports.ToolBuilderData{}, nil
	}

	var tools []ports.ToolBuilderData
	err := databases.MongoManager.FindAll(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.toolsBuilderCollectionName(),
		bson.M{"agent_type": agentType},
		&tools,
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}),
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			return []ports.ToolBuilderData{}, nil
		}
		return nil, err
	}

	if tools == nil {
		return []ports.ToolBuilderData{}, nil
	}

	return tools, nil
}

func (m *Manager) AddToolBuilder(tool ports.ToolBuilderData) error {
	tool = normalizeToolBuilderData(tool)
	if err := validateToolBuilderData(tool); err != nil {
		return err
	}

	filter := bson.M{"agent_type": tool.AgentType, "name": tool.Name}
	count, err := databases.MongoManager.CountDocument(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.toolsBuilderCollectionName(),
		filter,
	)
	if err != nil {
		return err
	}
	if count > 0 {
		return ports.ErrToolBuilderAlreadyExists
	}

	res, err := databases.MongoManager.InsertOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.toolsBuilderCollectionName(),
		tool,
	)
	if err != nil {
		return err
	}
	if res.InsertedID == nil {
		return ports.ErrInvalidToolBuilderData
	}

	return nil
}

func (m *Manager) UpdateToolBuilder(agentType, originalName string, tool ports.ToolBuilderData) error {
	agentType = normalizeToolBuilderText(agentType)
	originalName = normalizeToolBuilderText(originalName)
	tool = normalizeToolBuilderData(tool)
	if tool.AgentType == "" {
		tool.AgentType = agentType
	}
	if err := validateToolBuilderData(tool); err != nil {
		return err
	}
	if agentType == "" || originalName == "" {
		return ports.ErrInvalidToolBuilderData
	}

	if tool.AgentType != agentType || tool.Name != originalName {
		count, err := databases.MongoManager.CountDocument(
			m.config.Databases.MongoDB.DBAlias,
			m.config.Databases.MongoDB.DBName,
			m.toolsBuilderCollectionName(),
			bson.M{"agent_type": tool.AgentType, "name": tool.Name},
		)
		if err != nil {
			return err
		}
		if count > 0 {
			return ports.ErrToolBuilderAlreadyExists
		}
	}

	res, err := databases.MongoManager.ReplaceOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.toolsBuilderCollectionName(),
		bson.M{"agent_type": agentType, "name": originalName},
		tool,
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ports.ErrToolBuilderNotFound
	}

	return nil
}

func (m *Manager) DeleteToolBuilder(agentType, name string) error {
	agentType = normalizeToolBuilderText(agentType)
	name = normalizeToolBuilderText(name)
	if agentType == "" || name == "" {
		return ports.ErrInvalidToolBuilderData
	}

	res, err := databases.MongoManager.DeleteOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.toolsBuilderCollectionName(),
		bson.M{"agent_type": agentType, "name": name},
	)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ports.ErrToolBuilderNotFound
	}

	return nil
}
