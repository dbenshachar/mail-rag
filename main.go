package main

import (
	"context"
	"fmt"
	"log"
	"mail_rag/golang/api"
	"mail_rag/golang/env"
	"mail_rag/golang/mail"
	"mail_rag/golang/mongodb"
	"net/http"
	"os"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type service struct {
	client        *mongo.Client
	source        mail.LoopbackSource
	ollamaHost    string
	ollamaModel   string
	contextLength int
}

func main() {
	if err := env.LoadDotEnv(); err != nil {
		log.Fatal(err)
	}

	clientID := strings.TrimSpace(os.Getenv("gmail_client_id"))
	clientSecret := strings.TrimSpace(os.Getenv("gmail_secret"))
	redirectPort := strings.TrimSpace(os.Getenv("gmail_redirect"))
	mongoURI := strings.TrimSpace(os.Getenv("mongo_uri"))
	ollamaHost := strings.TrimSpace(os.Getenv("ollama_host"))
	ollamaModel := strings.TrimSpace(os.Getenv("ollama_model"))
	contextLengthRaw := strings.TrimSpace(os.Getenv("ollama_context"))
	frontendOrigin := strings.TrimSpace(os.Getenv("frontend_origin"))
	apiPort := strings.TrimSpace(os.Getenv("api_port"))

	if apiPort == "" {
		apiPort = "8080"
	}

	contextLength, err := strconv.Atoi(contextLengthRaw)
	if err != nil {
		log.Fatal(err)
	}

	mongoClient, err := mongodb.MongoClient(mongoURI)
	if err != nil {
		log.Fatal(err)
	}

	token, err := mail.GetInitialToken(clientID, clientSecret, redirectPort)
	if err != nil {
		log.Fatal(err)
	}

	source := mail.Make_Loopback_Source(*token, clientID, clientSecret)
	svc := &service{
		client:        mongoClient,
		source:        source,
		ollamaHost:    ollamaHost,
		ollamaModel:   ollamaModel,
		contextLength: contextLength,
	}

	httpServer := api.NewServer(svc, frontendOrigin)
	addr := ":" + apiPort
	fmt.Printf("API server running at http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, httpServer.Handler()); err != nil {
		log.Fatal(err)
	}
}

func (s *service) Sync(ctx context.Context) (int, error) {
	return mongodb.UpdateMongo(ctx, s.client, s.source, s.ollamaHost, s.ollamaModel, s.contextLength)
}

func (s *service) ListEmails(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error) {
	return mongodb.ListEmails(ctx, s.client, limit, offset)
}

func (s *service) Search(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error) {
	return mongodb.VectorSearch(ctx, s.client, "http://localhost:"+s.ollamaHost, s.ollamaModel, query, s.contextLength, threshold, limit)
}
