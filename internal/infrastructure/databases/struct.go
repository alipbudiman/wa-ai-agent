package databases

import (
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

var StoreOnce sync.Once

type MongoDBErrorMessage error

type MongoDBVar struct {
	Set  string `json:"$set"`
	Push string `json:"$push"`
	Pull string `json:"$pull"`
}

var MongoOPS = MongoDBVar{
	Set:  "$set",
	Push: "$push",
	Pull: "$pull",
}

var (
	ErrDocumentNotFound MongoDBErrorMessage = errors.New("document not found")

	MongoManager = &MongoConnections{
		Connections: make(map[string]*mongo.Client),
	}
)

type MongoConnections struct {
	Connections map[string]*mongo.Client
	Mu          sync.RWMutex
}
