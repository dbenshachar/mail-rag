package main

import (
	"log"
	"mail_rag/golang"
	"os"
)

func main() {
	golang.LoadDotEnv()
	clientID, clientSecret := os.Getenv("gmail_client_id"), os.Getenv("gmail_secret")

	token, err := golang.GetInitialToken(clientID, clientSecret, os.Getenv("gmail_redirect"))
	if err != nil {
		log.Fatal(err)
	}
	println(token.AccessToken)
}
