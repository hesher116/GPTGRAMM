package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"GPTGRAMM/internal/bot"
	"GPTGRAMM/internal/config"
)

func main() {
	cfg := config.LoadConfig()
	if cfg.TelegramToken == "" {
		log.Fatal("Не вказано токен Telegram бота")
	}

	bot, err := bot.NewBot(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Помилка створення бота: %v", err)
	}

	// Створюємо канал для отримання сигналів операційної системи
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Запускаємо бота в окремій горутині
	go bot.Start()

	// Очікуємо сигнал завершення
	<-sigChan
	log.Println("Отримано сигнал завершення, зупиняємо бота...")
} 