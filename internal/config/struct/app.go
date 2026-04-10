package cfg_struct

type ManagerConfig struct {
	BotConfigCollectionName   string
	AccessCollectionName      string
	TemplateMsgCollectionName string
	LogSystemCollectionName   string
	InboxCollectionName       string
}

type DynamicManagerConfig struct {
	RandID func() string
}

type AppConfig struct {
	Manager ManagerConfig
	Dynamic DynamicManagerConfig
}
