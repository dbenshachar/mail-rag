package golang

import (
	"context"
	"os"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func GetVector(message string) ([]float32, error) {
	ctx := context.Background()
	url := "http://localhost:" + os.Getenv("ollama_host")

	vec, err := GetEmbedding(ctx,
		url,
		os.Getenv("ollama_model"),
		message)

	return vec, err
}

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
	EmailID   string    `bson:"email_id"`
}
