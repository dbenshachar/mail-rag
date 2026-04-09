package mongodb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mail_rag/golang/mail"
	"mail_rag/golang/ollama"
	"os"
	"sort"
	"strings"
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
	Subject   string    `bson:"subject"`
	From      string    `bson:"from"`
	To        string    `bson:"to"`
	Date      time.Time `bson:"date"`
	Snippet   string    `bson:"snippet"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type EmailSummary struct {
	EmailID string    `json:"email_id" bson:"email_id"`
	Subject string    `json:"subject" bson:"subject"`
	From    string    `json:"from" bson:"from"`
	To      string    `json:"to" bson:"to"`
	Date    time.Time `json:"date" bson:"date"`
	Snippet string    `json:"snippet" bson:"snippet"`
}

type SearchResult struct {
	Score float32      `json:"score"`
	Email EmailSummary `json:"email"`
}

func BuildUpsertSpec(record mail.EmailRecord, embedding []float32, now time.Time) (bson.D, bson.D, *options.UpdateOneOptionsBuilder) {
	filter := bson.D{{Key: "email_id", Value: record.EmailID}}
	update := bson.D{{
		Key: "$set",
		Value: bson.D{
			{Key: "email_id", Value: record.EmailID},
			{Key: "subject", Value: record.Subject},
			{Key: "from", Value: record.From},
			{Key: "to", Value: record.To},
			{Key: "date", Value: record.Date},
			{Key: "snippet", Value: record.Snippet},
			{Key: "contents", Value: record.Contents},
			{Key: "embedding", Value: embedding},
			{Key: "updated_at", Value: now},
		},
	}}
	opts := options.UpdateOne().SetUpsert(true)
	return filter, update, opts
}

func UpsertEmbedding(ctx context.Context, client *mongo.Client, record mail.EmailRecord, embedding []float32, now time.Time) error {
	collection := client.Database("mail_rag").Collection("embeddings")
	filter, update, opts := BuildUpsertSpec(record, embedding, now)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	return err
}

func RankSearchResults(queryEmbedding []float32, docs []Document, threshold float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	results := make([]SearchResult, 0, len(docs))
	for _, doc := range docs {
		if len(doc.Embedding) != len(queryEmbedding) {
			continue
		}
		score, err := ollama.CosineSimilarity(queryEmbedding, doc.Embedding)
		if err != nil {
			return nil, err
		}
		if score < threshold {
			continue
		}
		results = append(results, SearchResult{
			Score: score,
			Email: EmailSummary{
				EmailID: doc.EmailID,
				Subject: doc.Subject,
				From:    doc.From,
				To:      doc.To,
				Date:    doc.Date,
				Snippet: doc.Snippet,
			},
		})
	}
	if len(results) == 0 {
		return results, nil
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Email.Date.After(results[j].Email.Date)
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func VectorSearch(
	ctx context.Context,
	client *mongo.Client,
	baseURL, model, query string,
	contextLength int,
	threshold float32,
	limit int,
) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SearchResult{}, nil
	}

	embed, err := ollama.GetEmbedding(ctx, baseURL, model, query, contextLength)
	if err != nil {
		return nil, err
	}

	col := client.Database("mail_rag").Collection("embeddings")
	cur, err := col.Find(
		ctx,
		bson.D{},
		options.Find().SetProjection(bson.M{
			"contents":   1,
			"embedding":  1,
			"email_id":   1,
			"subject":    1,
			"from":       1,
			"to":         1,
			"date":       1,
			"snippet":    1,
			"updated_at": 1,
		}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	docs := make([]Document, 0)
	for cur.Next(ctx) {
		var doc Document
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	return RankSearchResults(embed, docs, threshold, limit)
}

func ListEmails(ctx context.Context, client *mongo.Client, limit, offset int) ([]EmailSummary, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	col := client.Database("mail_rag").Collection("embeddings")
	findOpts := options.Find().
		SetProjection(bson.M{
			"email_id": 1,
			"subject":  1,
			"from":     1,
			"to":       1,
			"date":     1,
			"snippet":  1,
		}).
		SetSort(bson.D{{Key: "date", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cur, err := col.Find(ctx, bson.D{}, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	emails := make([]EmailSummary, 0, limit)
	for cur.Next(ctx) {
		var item EmailSummary
		if err := cur.Decode(&item); err != nil {
			return nil, 0, err
		}
		emails = append(emails, item)
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}

	total, err := col.CountDocuments(ctx, bson.D{})
	if err != nil {
		return nil, 0, err
	}

	return emails, total, nil
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

func UpdateMongo(ctx context.Context, client *mongo.Client, src mail.LoopbackSource, ollamaHost, ollamaModel string, contextLength int) (int, error) {
	_, err := mail.LoopbackRefresh(src)
	if err != nil {
		return 0, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	srv, err := mail.NewGmailService(ctx, src)
	if err != nil {
		return 0, err
	}

	date, err := LoadDateCache()
	if err != nil {
		date = mail.Date{Year: 2000, Month: 1, Day: 1}
	}

	ids, err := mail.FetchIDs(srv, date)
	if err != nil {
		return 0, err
	}

	if len(ids) == 0 {
		if err := WriteDateCache(GetCurrentDate()); err != nil {
			return 0, err
		}
		return 0, nil
	}

	records, err := mail.FetchMessages(srv, ids)
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	syncedCount := 0
	for _, record := range records {
		emb, err := ollama.GetEmbedding(ctx, "http://localhost:"+ollamaHost, ollamaModel, record.Contents, contextLength)
		if err != nil {
			return syncedCount, err
		}
		if err := UpsertEmbedding(ctx, client, record, emb, now); err != nil {
			return syncedCount, err
		}
		syncedCount++
	}

	if err := WriteDateCache(GetCurrentDate()); err != nil {
		return syncedCount, err
	}

	return syncedCount, nil
}
