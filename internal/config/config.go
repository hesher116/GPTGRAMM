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
	// Знаходимо кореневу директорію проекту
	projectRoot, err := findProjectRoot()
	if err != nil {
		log.Printf("Помилка пошуку кореневої директорії: %v", err)
	} else {
		// Завантажуємо .env з кореневої директорії
		envPath := filepath.Join(projectRoot, ".env")
		if err := godotenv.Load(envPath); err != nil {
			log.Printf("Помилка завантаження .env файлу: %v", err)
		}
	}

	return &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
	}
}

// findProjectRoot шукає кореневу директорію проекту
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Перевіряємо наявність go.mod файлу
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		// Переходимо на рівень вище
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
} 