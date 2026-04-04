package cfg_struct

type ManagerConfig struct {
	AccessCollectionName string
	BotConfigCollectionName string
}

type AppConfig struct {
	Manager ManagerConfig
}
