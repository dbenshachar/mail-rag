package golang

import (
	"encoding/json"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadDotEnv() error {
	err := godotenv.Load(".env")
	return err
}

type Config struct {
	API struct {
		RefreshBufferMinutes int `json:"refresh_buffer_minutes"`
	} `json:"api"`
}

func LoadJSON() (Config, error) {
	fileContent, err := os.ReadFile("constants.json")
	if err != nil {
		log.Fatal(err)
	}

	var config Config
	err = json.Unmarshal(fileContent, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}
