package golang

import (
	"encoding/json"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadDotEnv() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

type Config struct {
	API struct {
		RefreshBufferMinutes int `json:"refresh_buffer_minutes"`
	} `json:"api"`
}

func LoadJSON() Config {
	fileContent, err := os.ReadFile("constants.json")
	if err != nil {
		log.Fatal(err)
	}

	var config Config
	err = json.Unmarshal(fileContent, &config)
	if err != nil {
		log.Fatal(err)
	}
	return config
}
