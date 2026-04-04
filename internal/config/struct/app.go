package cfg_struct

type ManagerConfig struct {
	AccessCollectionName string
}

type AppConfig struct {
	Manager ManagerConfig
}
