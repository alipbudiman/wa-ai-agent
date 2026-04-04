package manager

import (
	"context"
	"time"

	appconfig "github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"go.mau.fi/whatsmeow"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Manager struct {
	client   *whatsmeow.Client
	waclient ports.WhatsAppClient
	config   appconfig.Config
}

func NewManager(client *whatsmeow.Client, waclient ports.WhatsAppClient, config appconfig.Config) (*Manager, error) {
	return &Manager{
		client:   client,
		waclient: waclient,
		config:   config,
	}, nil
}

func (m *Manager) getCurrentTime() int64 {
	return time.Now().Unix()
}

func (m *Manager) CheckAccess(number string) (bool, error) {
	filter := bson.M{"number": number}
	var access_info AccessInfo
	err := databases.MongoManager.FindOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.AccessCollectionName,
		filter,
		&access_info,
	)
	if err != nil {
		return false, err
	}

	if access_info.ExpiredAt < m.getCurrentTime() {
		m.DeleteAccess(number)
		return false, nil
	}

	return true, nil
}

func (m *Manager) GetAccessList() ([]ports.AccessInfo, error) {
	var rows []AccessInfo
	err := databases.MongoManager.FindAll(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.AccessCollectionName,
		bson.M{},
		&rows,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			return []ports.AccessInfo{}, nil
		}
		return nil, err
	}

	result := make([]ports.AccessInfo, 0, len(rows))
	for _, row := range rows {
		result = append(result, ports.AccessInfo{
			Name:      row.Name,
			Number:    row.Number,
			CreatedAt: row.CreatedAt,
			ExpiredAt: row.ExpiredAt,
			JID:       row.JID.String(),
		})
	}

	return result, nil
}

func (m *Manager) AddAccess(name, number string, expiredAt int64) error {
	contact_info, err := m.waclient.GetContact(context.Background(), number)
	if err != nil {
		return err
	}

	if !contact_info.Found {
		return ErrContactNotFound
	}

	filter := bson.M{"number": number}

	count, err := databases.MongoManager.CountDocument(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.AccessCollectionName,
		filter,
	)

	if err != nil {
		return err
	}
	
	if count > 0 {
		return ErrAccessAlreadyExists
	}
	
	new_jid, err := m.waclient.ParseJID(number)
	if err != nil {
		return err
	}

	res, err := databases.MongoManager.InsertOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.AccessCollectionName,
		AccessInfo{
			Name:      name,
			Number:    number,
			CreatedAt: m.getCurrentTime(),
			ExpiredAt: expiredAt,
			JID:       new_jid,
		},
	)

	if err != nil {
		return err
	}

	if res.InsertedID == nil {
		return ErrFailedToAddAccess
	}

	return nil
}


func (m *Manager) DeleteAccess(number string) error {
	filter := bson.M{"number": number}
	_, err := databases.MongoManager.DeleteOne(
		m.config.Databases.MongoDB.DBAlias,
		m.config.Databases.MongoDB.DBName,
		m.config.App.Manager.AccessCollectionName,
		filter,
	)
	return err
}
