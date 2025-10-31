package main

import (
	"fmt"
	"log"
	"mail_rag/golang/env"
	"mail_rag/golang/mail"
	"mail_rag/golang/mongodb"
	"os"
	"strconv"
)

func main() {
	env.LoadDotEnv()

	clientID, clientSecret := os.Getenv("gmail_client_id"), os.Getenv("gmail_secret")

	fmt.Println("Getting token...")
	token, err := mail.GetInitialToken(clientID, clientSecret, os.Getenv("gmail_redirect"))
	fmt.Println("Got token!")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Making source...")
	src := mail.Make_Loopback_Source(*token, clientID, clientSecret)
	_, err = mail.LoopbackRefresh(src)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Made source!")
	fmt.Println("Making client...")
	client, err := mongodb.MongoClient(os.Getenv("mongo_uri"))
	fmt.Println("Made client!")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Getting emails...")

	contextLength, err := strconv.Atoi((os.Getenv("ollama_context")))
	if err != nil {
		log.Fatal(err)
	}
	err = mongodb.UpdateMongo(client, src, os.Getenv("ollama_host"), os.Getenv("ollama_model"), contextLength)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Added emails to Mongo!")
}
