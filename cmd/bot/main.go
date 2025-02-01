package main

import (
	"GPTGRAMM/internal/bot"
	"GPTGRAMM/internal/config"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := config.LoadConfig()
	if cfg.TelegramToken == "" {
		log.Fatal("Не вказано токен Telegram бота")
	}

	myBot, err := bot.NewBot(cfg.TelegramToken)
	if err != nil {

		log.Fatalf("Помилка створення бота: %v", err)
	}

	// Канал для отримання сигналів операційної системи
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Запускаємо бота в окремій горутині
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer myBot.Storage.Close()

	go myBot.Start(ctx)

	// Очікуємо сигнал завершення
	<-sigChan
	log.Println("Отримано сигнал завершення, зупиняємо бота...")
}
