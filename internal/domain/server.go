package ports

import (
	"go.mau.fi/whatsmeow/types"
)

type AccessInfo struct {
	Name      string `json:"name" bson:"name"`
	Number    string `json:"number" bson:"number"`
	CreatedAt int64  `json:"created_at" bson:"created_at"`
	ExpiredAt int64  `json:"expired_at" bson:"expired_at"`
	JID       string `json:"jid" bson:"jid"`
}

type InboxMessage struct {
	JID        types.JID `json:"jid" bson:"jid"`
	MsgID      string    `json:"msg_id" bson:"msg_id"`
	FullName   string    `json:"name" bson:"name"`
	PushName   string    `json:"push_name" bson:"push_name"`
	Number     string    `json:"number" bson:"number"`
	Message    string    `json:"message" bson:"message"`
	ReceivedAt int64     `json:"received_at" bson:"received_at"`
	FromMe     bool      `json:"from_me" bson:"from_me"`
	Read       bool      `json:"read" bson:"read"`
}

type Manager interface {
	CheckAccess(number string) (bool, error)
	GetAccessList() ([]AccessInfo, error)
	AddAccess(name, number string, expiredAt int64) error
	DeleteAccess(number string) error
	GetInboxAddress() (types.JID, error)
	SetInboxAddress(number string) error
	ClearInboxAddress() error
	GetInboxMessages() ([]InboxMessage, error)
	DeleteMessage(msg_id string) error
	DeleteAllInboxMessagesByJID(jid types.JID) error
	DeleteAllInboxMessages() error
	StoreInboxMessage(message InboxMessage) error
	GetUnreadInboxMessages(jid types.JID) ([]string, error)
	MarkInboxMessageAsRead(msg_id string) error
}
