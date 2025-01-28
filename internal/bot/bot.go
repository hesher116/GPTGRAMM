package bot

import (
	"fmt"
	"log"
	"GPTGRAMM/internal/api"
	"GPTGRAMM/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot представляє основну структуру телеграм бота
type Bot struct {
	api     *tgbotapi.BotAPI
	storage *storage.Storage
	chatGPT *api.ChatGPT
}

// NewBot створює новий екземпляр бота
func NewBot(token string) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("помилка створення бота: %w", err)
	}

	storage, err := storage.NewStorage()
	if err != nil {
		return nil, fmt.Errorf("помилка ініціалізації сховища: %w", err)
	}

	return &Bot{
		api:     bot,
		storage: storage,
	}, nil
}

// Start запускає бота та починає обробку повідомлень
func (b *Bot) Start() {
	log.Printf("Бот %s запущено", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go b.handleMessage(update.Message)
	}
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	text := message.Text

	switch text {
	case "/start":
		b.handleStart(chatID)
	case "/help":
		b.handleHelp(chatID)
	case "/change_key":
		b.handleChangeKey(chatID)
	default:
		if len(text) > 3 && text[:3] == "sk-" {
			b.handleAPIKey(chatID, text)
		} else {
			b.handleGPTRequest(chatID, text)
		}
	}
}

func (b *Bot) handleStart(chatID int64) {
	text := `👋 Вітаю! Я бот, створений @hesher116, який допоможе вам спілкуватися з ChatGPT напряму через ваш OpenAI API ключ.

Для початку роботи, будь ласка, надішліть свій OpenAI API ключ.
Якщо у вас його немає, отримайте на сайті: https://platform.openai.com/account/api-keys

Для допомоги використовуйте команду /help`

	b.sendMessage(chatID, text)
}

func (b *Bot) handleHelp(chatID int64) {
	text := `📌 Доступні команди:

/start - Почати роботу
/help - Показати це повідомлення
/change_key - Змінити API ключ

Просто надішліть повідомлення, і я передам його до ChatGPT!`

	b.sendMessage(chatID, text)
}

func (b *Bot) handleChangeKey(chatID int64) {
	text := "🔑 Будь ласка, надішліть новий API ключ"
	b.sendMessage(chatID, text)
}

func (b *Bot) handleAPIKey(chatID int64, apiKey string) {
	if err := b.storage.SaveAPIKey(chatID, apiKey); err != nil {
		b.sendMessage(chatID, "❌ Помилка збереження ключа. Спробуйте ще раз.")
		return
	}

	b.sendMessage(chatID, "✅ API ключ успішно збережено! Тепер ви можете надсилати повідомлення.")
}

func (b *Bot) handleGPTRequest(chatID int64, text string) {
	apiKey, err := b.storage.GetAPIKey(chatID)
	if err != nil {
		b.sendMessage(chatID, "❌ Будь ласка, спочатку надішліть свій API ключ.")
		return
	}

	gpt := api.NewChatGPT(apiKey)
	response, err := gpt.SendMessage(text)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Помилка: %v", err))
		return
	}

	b.sendMessage(chatID, response)
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown

	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
} 