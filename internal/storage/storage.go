package storage

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

func NewStorage() (*Storage, error) {
	db, err := sql.Open("sqlite", "bot.db")
	if err != nil {
		return nil, fmt.Errorf("помилка підключення до бази даних: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("помилка перевірки з'єднання з базою даних: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("помилка створення таблиць: %w", err)
	}

	return &Storage{db: db}, nil
}

func createTables(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		chat_id INTEGER PRIMARY KEY,
		api_key TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	_, err := db.Exec(query)
	return err
}

func (s *Storage) SaveAPIKey(chatID int64, apiKey string) error {
	query := `
	INSERT INTO users (chat_id, api_key) 
	VALUES (?, ?) 
	ON CONFLICT(chat_id) DO UPDATE SET api_key = excluded.api_key`
	
	_, err := s.db.Exec(query, chatID, apiKey)
	return err
}

func (s *Storage) GetAPIKey(chatID int64) (string, error) {
	var apiKey string
	err := s.db.QueryRow("SELECT api_key FROM users WHERE chat_id = ?", chatID).Scan(&apiKey)
	return apiKey, err
} 