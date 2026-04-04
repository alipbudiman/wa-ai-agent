package ports

type AccessInfo struct {
	Name      string `json:"name" bson:"name"`
	Number    string `json:"number" bson:"number"`
	CreatedAt int64  `json:"created_at" bson:"created_at"`
	ExpiredAt int64  `json:"expired_at" bson:"expired_at"`
	JID       string `json:"jid" bson:"jid"`
}

type Manager interface {
	CheckAccess(number string) (bool, error)
	GetAccessList() ([]AccessInfo, error)
	AddAccess(name, number string, expiredAt int64) error
	DeleteAccess(number string) error
}
