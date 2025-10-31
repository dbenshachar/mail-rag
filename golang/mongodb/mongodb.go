package mongodb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mail_rag/golang/mail"
	"mail_rag/golang/ollama"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func MongoClient(mongoURI string) (*mongo.Client, error) {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(mongoURI).SetServerAPIOptions(serverAPI)
	client, err := mongo.Connect(opts)

	if err != nil {
		return nil, err
	}

	if err := client.Ping(context.TODO(), readpref.Primary()); err != nil {
		return nil, err
	}
	return client, nil
}

type Document struct {
	Contents  string    `bson:"contents"`
	Embedding []float32 `bson:"embedding"`
	EmailID   string    `bson:"email_id"`
}

func InsertEmbedding(client *mongo.Client, embedding []float32, contents, id string) error {
	collection := client.Database("mail_rag").Collection("embeddings")

	doc := Document{
		Contents:  contents,
		Embedding: embedding,
		EmailID:   id,
	}

	_, err := collection.InsertOne(context.TODO(), doc)
	if err != nil {
		return err
	}

	return nil
}

func VectorSearch(
	ctx context.Context,
	client *mongo.Client,
	baseURL, model, query string,
	contextLength int,
	threshold float32,
) ([]string, error) {

	embed, err := ollama.GetEmbedding(ctx, baseURL, model, query, contextLength)
	if err != nil {
		return nil, err
	}

	col := client.Database("mail_rag").Collection("embeddings")
	cur, err := col.Find(
		ctx,
		bson.D{},
		options.Find().SetProjection(bson.M{
			"contents":  1,
			"embedding": 1,
			"email_id":  1,
		}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []string
	for cur.Next(ctx) {
		var doc Document
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}

		if len(doc.Embedding) != len(embed) {
			continue
		}

		score, err := ollama.CosineSimilarity(embed, doc.Embedding)
		if err != nil {
			return nil, err
		}
		if score >= threshold {
			out = append(out, doc.Contents)
		}
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func LoadDateCache() (mail.Date, error) {
	const filePath = ".data/mongo_cache.json"
	if err := os.MkdirAll(".data", 0700); err != nil {
		return mail.Date{}, err
	}

	b, err := os.ReadFile(filePath)
	if err != nil {
		return mail.Date{}, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return mail.Date{}, errors.New("date cache file is empty")
	}

	var date mail.Date
	if err := json.Unmarshal(b, &date); err != nil {
		return mail.Date{}, fmt.Errorf("invalid date cache: %w", err)
	}

	if date.Year <= 0 || date.Month <= 0 || date.Month > 12 || date.Day <= 0 || date.Day > 31 {
		return mail.Date{}, errors.New("invalid date in cache")
	}

	return date, nil
}

func WriteDateCache(date mail.Date) error {
	if err := os.MkdirAll(".data", 0700); err != nil {
		return err
	}

	if date.Year <= 0 || date.Month <= 0 || date.Month > 12 || date.Day <= 0 || date.Day > 31 {
		return errors.New("invalid date")
	}

	data, err := json.MarshalIndent(date, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(".data/mongo_cache.json", data, 0600)
}

func GetCurrentDate() mail.Date {
	now := time.Now()
	return mail.Date{
		Year:  now.Year(),
		Month: int(now.Month()),
		Day:   now.Day(),
	}
}

func UpdateMongo(client *mongo.Client, src mail.LoopbackSource, ollama_host, ollama_model string, contextLength int) error {
	_, err := mail.LoopbackRefresh(src)
	if err != nil {
		return err
	}

	ctx := context.Background()
	srv, err := mail.NewGmailService(ctx, src)
	if err != nil {
		log.Fatal(err)
	}

	date, err := LoadDateCache()
	if err != nil {
		log.Fatal(err)
	}

	ids, err := mail.FetchIDs(srv, date)
	if err != nil {
		log.Fatal(err)
	}

	contents, err := mail.FetchMessages(srv, ids)
	if err != nil {
		log.Fatal(err)
	}

	for idx := range contents {
		emb, err := ollama.GetEmbedding(ctx, "http://localhost:"+ollama_host, ollama_model, contents[idx], contextLength)
		if err == nil {
			err = InsertEmbedding(client, emb, contents[idx], ids[idx])
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	WriteDateCache(GetCurrentDate())

	return err
}
