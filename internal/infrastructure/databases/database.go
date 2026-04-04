package databases

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (mc *MongoConnections) ConnectToMongoDB(dbAlias, uri string) error {
	mc.Mu.Lock()
	defer mc.Mu.Unlock()

	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return err
	}

	// Check the connection
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return err
	}

	mc.Connections[dbAlias] = client

	return nil
}

func (mc *MongoConnections) ensureDatabaseAndCollection(dbAlias string, ctx context.Context, dbName, collectionName string) (*mongo.Client, error) {
	client, exists := mc.Connections[dbAlias]
	if !exists {
		return nil, fmt.Errorf("connection %s not found", dbAlias)
	}

	collections, err := client.Database(dbName).ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %v", err)
	}

	for _, name := range collections {
		if name == collectionName {
			return client, nil
		}
	}

	// Create the collection explicitly
	err = client.Database(dbName).CreateCollection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %v", err)
	}

	return client, nil
}

// publict

func (mc *MongoConnections) GetCollection(dbAlias string, db_name string, db_coll string) (*mongo.Collection, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	MongoClient, err := mc.ensureDatabaseAndCollection(dbAlias, ctx, db_name, db_coll)
	if err != nil {
		return nil, nil, cancel, err
	}

	db := MongoClient.Database(db_name).Collection(db_coll)
	return db, ctx, cancel, nil
}

func (mc *MongoConnections) InsertOne(dbAlias string, db_name string, db_coll string, data interface{}) (*mongo.InsertOneResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	insert_res, err := db.InsertOne(ctx, data)
	if err != nil {
		return nil, err
	}
	return insert_res, nil
}

func (mc *MongoConnections) InsertMany(dbAlias string, db_name string, db_coll string, data []interface{}) (*mongo.InsertManyResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	insert_res, err := db.InsertMany(ctx, data)
	if err != nil {
		return nil, err
	}
	return insert_res, nil
}

func (mc *MongoConnections) UpdateOne(dbAlias string, db_name string, db_coll string, filter interface{}, update interface{}) (*mongo.UpdateResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	update_res, err := db.UpdateOne(ctx, filter, update)
	if err != nil {
		return nil, err
	}
	return update_res, nil
}

func (mc *MongoConnections) UpdateMany(dbAlias string, db_name string, db_coll string, filter interface{}, update interface{}) (*mongo.UpdateResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	update_res, err := db.UpdateMany(ctx, filter, update)
	if err != nil {
		return nil, err
	}
	return update_res, nil
}

func (mc *MongoConnections) ReplaceOne(dbAlias string, db_name string, db_coll string, filter interface{}, update interface{}) (*mongo.UpdateResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	replace_res, err := db.ReplaceOne(ctx, filter, update)
	if err != nil {
		return nil, err
	}
	return replace_res, nil
}

func (mc *MongoConnections) DeleteOne(dbAlias string, db_name string, db_coll string, filter interface{}) (*mongo.DeleteResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	delete_res, err := db.DeleteOne(ctx, filter)
	if err != nil {
		return nil, err
	}
	return delete_res, nil
}

func (mc *MongoConnections) DeleteMany(dbAlias string, db_name string, db_coll string, filter interface{}) (*mongo.DeleteResult, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return nil, err
	}
	defer cancel()
	delete_res, err := db.DeleteMany(ctx, filter)
	if err != nil {
		return nil, err
	}
	return delete_res, nil
}

/*
return results with pointer &results
*/
func (mc *MongoConnections) FindOne(dbAlias string, db_name string, db_coll string, filter any, results any, opts ...options.Lister[options.FindOneOptions]) error {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return err
	}
	defer cancel()

	err = db.FindOne(ctx, filter, opts...).Decode(results)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrDocumentNotFound
		}
		return err
	}
	return nil
}

/*
return results with pointer &results
*/
func (mc *MongoConnections) FindAll(dbAlias string, db_name string, db_coll string, filter any, results any, opts ...options.Lister[options.FindOptions]) error {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return err
	}
	defer cancel()
	find_res, err := db.Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	if err = find_res.All(context.TODO(), results); err != nil {
		return err
	}
	return nil
}

func (mc *MongoConnections) CountDocument(dbAlias string, db_name string, db_coll string, filter interface{}) (int64, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return -1, err
	}
	defer cancel()
	count, err := db.CountDocuments(ctx, filter)
	if err != nil {
		return -1, err
	}

	return count, nil
}

func (mc *MongoConnections) CountDocuments(dbAlias string, db_name string, db_coll string, filter interface{}) (int64, error) {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return 0, err
	}

	defer cancel()
	count, err := db.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}

/*
MongoDBAggregate runs an aggregation pipeline and decodes the results into the provided slice pointer.
The results parameter must be a pointer to a slice (e.g., &[]YourStruct{}).
*/
func (mc *MongoConnections) Aggregate(dbAlias string, db_name string, db_coll string, pipeline mongo.Pipeline, results any) error {
	db, ctx, cancel, err := mc.GetCollection(dbAlias, db_name, db_coll)
	if err != nil {
		return err
	}
	defer cancel()

	cursor, err := db.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	if err = cursor.All(ctx, results); err != nil {
		return err
	}
	return nil
}
