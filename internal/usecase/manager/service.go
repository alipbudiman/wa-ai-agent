package manager

import (
	appconfig "github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow"
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
