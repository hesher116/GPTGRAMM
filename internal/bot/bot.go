package bot

import (
	"GPTGRAMM/internal/storage"
	"context"
	"fmt"
	"log"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api        *tgbotapi.BotAPI
	Storage    *storage.Storage
	chatGPTs   sync.Map
	users      sync.Map
	modelCache map[int64]string 
	messageIDs sync.Map
}

func NewBot(token string) (*Bot, error) {
	myBot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("помилка створення бота: %w", err)
	}

	storage, err := storage.NewStorage()
	if err != nil {
		return nil, fmt.Errorf("помилка ініціалізації сховища: %w", err)
	}

	return &Bot{
		api:        myBot,
		Storage:    storage,
		chatGPTs:   sync.Map{},
		messageIDs: sync.Map{},
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	log.Printf("бот %s запущено", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)
	workerPool := make(chan struct{}, 10)

	for {
		select {
		case <-ctx.Done():
			log.Println("сигнал завершення, зупиняємо роботу...")
			return
		case update, ok := <-updates:
			if !ok {
				log.Println("канал оновлень закритий, вихід...")
				return
			}

			workerPool <- struct{}{} // Блокує, якщо всі потоки зайняті
			go func(update tgbotapi.Update) {
				defer func() { <-workerPool }() // Звільняє воркера після завершення

				if update.CallbackQuery != nil {
					b.handleCallback(update.CallbackQuery)
				} else if update.Message != nil {
					b.handleMessage(update.Message)
				}
			}(update)
		}
	}
}

func (b *Bot) sendMessage(chatID int64, text string, markdown ...bool) {
	msg := tgbotapi.NewMessage(chatID, text)
	if len(markdown) > 0 && markdown[0] {
		msg.ParseMode = tgbotapi.ModeMarkdown
	}

	if msg.ReplyMarkup == nil {
		msg.ReplyMarkup = createMainKeyboard()
	}

	sent, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return
	}

	queue := b.getMessageQueue(chatID)
	queue.Add(sent.MessageID)
}

func (b *Bot) getMessageQueue(chatID int64) *MessageQueue {
	queue, _ := b.messageIDs.LoadOrStore(chatID, NewMessageQueue(100))
	return queue.(*MessageQueue)
	
}
