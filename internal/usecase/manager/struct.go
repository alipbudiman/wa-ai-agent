package manager

import (
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

var (
	ErrContactNotFound           = fmt.Errorf("contact not found in WhatsApp")
	ErrAccessAlreadyExists       = fmt.Errorf("access already exists")
	ErrFailedToAddAccess         = fmt.Errorf("failed to add access")
	ErrInboxSetupNotFound        = fmt.Errorf("inbox setup not found")
	ErrFailedToStoreInboxMessage = fmt.Errorf("failed to store inbox message")
	ErrInboxMessageNotFound      = fmt.Errorf("inbox message not found")
	ErrFailedToAddProviders      = fmt.Errorf("failed to add providers")
	EmptyJID                     = types.JID{}
)

type AccessInfo struct {
	Name      string    `bson:"name"`
	Number    string    `bson:"number"`
	CreatedAt int64     `bson:"created_at"`
	ExpiredAt int64     `bson:"expired_at"`
	JID       types.JID `bson:"jid"`
}

type InboxSetupInfo struct {
	JID       types.JID `bson:"jid"`
	Parameter string    `bson:"parameter"`
}

type ProviderInfo struct {
	ConfigName string `bson:"config_name"`
	Providers  string `bson:"providers"`
}
