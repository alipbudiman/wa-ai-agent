package cfg_struct

type BotInfoConfig struct {
	Name        string `bson:"name" json:"name"`
	Number	  string `bson:"number" json:"number"`
}