package golang

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func MongoClient(mongoURI string) (*mongo.Client, error) {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(mongoURI).SetServerAPIOptions(serverAPI)
	client, err := mongo.Connect(opts)

	if err := client.Ping(context.TODO(), readpref.Primary()); err != nil {
		return nil, err
	}
	return client, err
}

type Document struct {
	Contents  string    `bson:"contents"`
	Embedding []float32 `bson:"embedding"`
}
