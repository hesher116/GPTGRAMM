package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken string
}

func LoadConfig() *Config {
	projectRoot, err := findProjectRoot()
	if err != nil {
		log.Printf("Помилка пошуку кореневої директорії: %v", err)
	} else {
		envPath := filepath.Join(projectRoot, ".env")
		if err := godotenv.Load(envPath); err != nil {
			log.Printf("Помилка завантаження .env файлу: %v", err)
		}
	}
	
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramToken == "" {
		log.Fatal("❌ ПОМИЛКА: TELEGRAM_TOKEN не знайдено! Переконайтеся, що він є у .env або середовищі")
	}
	return &Config{
		TelegramToken: telegramToken,
	}
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
