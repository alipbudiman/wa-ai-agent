package manager

import (
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

var (
	ErrContactNotFound     = fmt.Errorf("contact not found in WhatsApp")
	ErrAccessAlreadyExists = fmt.Errorf("access already exists")
	ErrFailedToAddAccess   = fmt.Errorf("failed to add access")
)

type AccessInfo struct {
	Name      string    `bson:"name"`
	Number    string    `bson:"number"`
	CreatedAt int64     `bson:"created_at"`
	ExpiredAt int64     `bson:"expired_at"`
	JID       types.JID `bson:"jid"`
}
