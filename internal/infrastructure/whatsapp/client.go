package whatsapp

import (
	"errors"

	"go.mau.fi/whatsmeow"
)

type BotClient struct {
	client *whatsmeow.Client
}

func NewBotClient(client *whatsmeow.Client) (*BotClient, error) {
	if client == nil {
		return nil, errors.New("whatsapp client is required")
	}

	return &BotClient{client: client}, nil
}
