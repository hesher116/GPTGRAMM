package storage

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
	"log"
	"GPTGRAMM/internal/api"
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
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			chat_id INTEGER PRIMARY KEY,
			api_key TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS chat_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER,
			message TEXT,
			response TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_settings (
			chat_id INTEGER PRIMARY KEY,
			model TEXT NOT NULL DEFAULT 'gpt-3.5-turbo',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
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

func (s *Storage) ClearHistory(chatID int64) error {
	log.Printf("Починаємо очищення історії для користувача %d", chatID)

	// Спочатку перевіряємо, чи є історія для цього користувача
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM chat_history WHERE chat_id = ?", chatID).Scan(&count)
	if err != nil {
		return fmt.Errorf("помилка перевірки історії: %w", err)
	}

	// Видаляємо всі повідомлення для цього користувача
	result, err := s.db.Exec("DELETE FROM chat_history WHERE chat_id = ?", chatID)
	if err != nil {
		return fmt.Errorf("помилка видалення історії: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("помилка отримання кількості видалених рядків: %w", err)
	}

	log.Printf("Видалено %d повідомлень для користувача %d", rowsAffected, chatID)
	return nil
}

func (s *Storage) SaveToHistory(chatID int64, message, response string) error {
	_, err := s.db.Exec(`
		INSERT INTO chat_history (chat_id, message, response) 
		VALUES (?, ?, ?)
	`, chatID, message, response)
	return err
}

func (s *Storage) GetHistory(chatID int64) ([]struct {
	Message   string
	Response  string
	CreatedAt time.Time
}, error) {
	rows, err := s.db.Query(`
		SELECT message, response, created_at 
		FROM chat_history 
		WHERE chat_id = ? 
		ORDER BY created_at DESC 
		LIMIT 10
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []struct {
		Message   string
		Response  string
		CreatedAt time.Time
	}

	for rows.Next() {
		var h struct {
			Message   string
			Response  string
			CreatedAt time.Time
		}
		if err := rows.Scan(&h.Message, &h.Response, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	return history, nil
}

func (s *Storage) SaveUserSettings(chatID int64, model string) error {
	log.Printf("Зберігаємо налаштування для користувача %d: модель %s", chatID, model)
	
	_, err := s.db.Exec(`
		INSERT INTO user_settings (chat_id, model) 
		VALUES (?, ?) 
		ON CONFLICT(chat_id) DO UPDATE SET 
			model = excluded.model,
			updated_at = CURRENT_TIMESTAMP
	`, chatID, model)
	
	if err != nil {
		log.Printf("Помилка збереження налаштувань: %v", err)
	}
	return err
}

func (s *Storage) GetUserSettings(chatID int64) (string, error) {
	var model string
	err := s.db.QueryRow("SELECT model FROM user_settings WHERE chat_id = ?", chatID).Scan(&model)
	
	if err == sql.ErrNoRows {
		log.Printf("Налаштування не знайдено для користувача %d, використовуємо модель за замовчуванням", chatID)
		return api.ModelGPT3, nil
	}
	
	if err != nil {
		log.Printf("Помилка отримання налаштувань: %v", err)
		return api.ModelGPT3, err
	}

	log.Printf("Отримано налаштування для користувача %d: модель %s", chatID, model)
	return model, nil
}

func (s *Storage) HasHistory(chatID int64) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM chat_history WHERE chat_id = ?", chatID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
} 