package main

import (
	"context"
	"log"
	"mail_rag/golang/env"
	"mail_rag/golang/mail"
	"mail_rag/golang/ollama"
	"os"
)

func main() {
	env.LoadDotEnv()

	clientID, clientSecret := os.Getenv("gmail_client_id"), os.Getenv("gmail_secret")

	token, err := mail.GetInitialToken(clientID, clientSecret, os.Getenv("gmail_redirect"))
	if err != nil {
		log.Fatal(err)
	}

	src := mail.Make_Loopback_Source(*token, clientID, clientSecret)
	ctx := context.Background()
	srv, err := mail.NewGmailService(ctx, src)
	if err != nil {
		log.Fatal(err)
	}

	date := mail.Make_Date(2025, 10, 28)

	ids, err := mail.FetchIDs(srv, date)
	if err != nil {
		log.Fatal(err)
	}

	contents, err := mail.FetchMessages(srv, ids)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Retrieved %d messages, first message length: %d", len(contents), len(contents[0]))

	emb, err := ollama.GetEmbedding(ctx, "http://localhost:"+os.Getenv("ollama_host"), os.Getenv("ollama_model"), contents[0])
	if err != nil {
		log.Fatal(err)
	}

	print(emb[0])
}
