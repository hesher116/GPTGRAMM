package storage

import (
	"database/sql"
	"fmt"
	"log"
	_ "modernc.org/sqlite"
	"sync"
	"time"
)

type Storage struct {
	db         *sql.DB
	modelCache map[int64]string // ✅ Додаємо кеш
	mu         sync.RWMutex     // ✅ Додаємо м'ютекс
}

var settingsCache = sync.Map{}

func NewStorage() (*Storage, error) {
	db, err := sql.Open("sqlite", "bot.db")
	if err != nil {
		return nil, fmt.Errorf("❌ Помилка підключення до бази: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("❌ Помилка перевірки підключення: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("❌ Помилка створення таблиць: %w", err)
	}

	return &Storage{
		db:         db,
		modelCache: make(map[int64]string),
		mu:         sync.RWMutex{},
	}, nil
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
	stmt, err := s.db.Prepare(`
		INSERT INTO users (chat_id, api_key) 
		VALUES (?, ?) 
		ON CONFLICT(chat_id) DO UPDATE SET api_key = excluded.api_key`)
	if err != nil {
		log.Printf("Помилка підготовки SQL-запиту: %v", err)
		return err
	}
	defer stmt.Close() // Гарантовано закриваємо запит після виконання

	_, err = stmt.Exec(chatID, apiKey)
	if err != nil {
		log.Printf("Помилка виконання SQL-запиту: %v", err)
	}
	return err
}

func (s *Storage) GetAPIKey(chatID int64) (string, error) {
	var apiKey string
	err := s.db.QueryRow("SELECT api_key FROM users WHERE chat_id = ?", chatID).Scan(&apiKey)
	return apiKey, err
}

func (s *Storage) ClearHistory(chatID int64) error {
	log.Printf("Починаємо очищення історії для користувача %d", chatID)

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM chat_history WHERE chat_id = ?", chatID).Scan(&count)
	if err != nil {
		return fmt.Errorf("помилка перевірки історії: %w", err)
	}

	result, err := s.db.Exec("DELETE FROM chat_history WHERE chat_id = ?", chatID)
	if err != nil {
		return fmt.Errorf("помилка видалення історії: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("помилка отримання кількості видалених рядків: %w", err)
	}

	log.Printf("Видалено %d повідомлень для користувача %d з бази данних", rowsAffected, chatID)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	s.modelCache[chatID] = model // Оновлюємо кеш
	_, err := s.db.Exec(`
		INSERT INTO user_settings (chat_id, model) 
		VALUES (?, ?) 
		ON CONFLICT(chat_id) DO UPDATE SET 
			model = excluded.model,
			updated_at = CURRENT_TIMESTAMP
	`, chatID, model)

	if err != nil {
		log.Printf("❌ Помилка збереження налаштувань моделі: %v", err)
	}
	return err
}

func (s *Storage) GetUserSettings(chatID int64) (string, error) {
	s.mu.RLock()
	if model, ok := s.modelCache[chatID]; ok {
		s.mu.RUnlock()
		return model, nil
	}
	s.mu.RUnlock()

	// Якщо немає у кеші, беремо з бази
	var model string
	err := s.db.QueryRow("SELECT model FROM user_settings WHERE chat_id = ?", chatID).Scan(&model)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("❌ Помилка отримання налаштувань: %w", err)
	}

	// Зберігаємо у кеш
	s.mu.Lock()
	s.modelCache[chatID] = model
	s.mu.Unlock()

	return model, nil
}

func (s *Storage) HasHistory(chatID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM chat_history WHERE chat_id = ?)", chatID).Scan(&exists)
	return exists, err
}

func (s *Storage) Close() error {
	log.Println("Закривається підключення до бази даних...")
	return s.db.Close()
}
