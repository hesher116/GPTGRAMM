package bot

import (
	"fmt"
	"log"
	"GPTGRAMM/internal/api"
	"GPTGRAMM/internal/storage"
	"time"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Додаємо нові типи та константи
const (
	maxRequestsPerDay = 10
	bypassCode       = "UNLIMITED_ACCESS"  // Кодова фраза для обходу ліміту
)

// Додаємо структуру для налаштувань користувача
type UserSettings struct {
	Model       string
	RequestCount int
	LastRequest  time.Time
}

// Bot представляє основну структуру телеграм бота
type Bot struct {
	api         *tgbotapi.BotAPI
	storage     *storage.Storage
	chatGPTs    map[int64]*api.ChatGPT
	users       map[int64]*UserSettings
	messageIDs  map[int64][]int // Зберігаємо всі ID повідомлень для кожного чату
	mu          sync.RWMutex
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
		api:         bot,
		storage:     storage,
		chatGPTs:    make(map[int64]*api.ChatGPT),
		users:       make(map[int64]*UserSettings),
		messageIDs:  make(map[int64][]int),
	}, nil
}

// Start запускає бота та починає обробку повідомлень
func (b *Bot) Start() {
	log.Printf("Бот %s запущено", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			go b.handleCallback(update.CallbackQuery)
		} else if update.Message != nil {
			go b.handleMessage(update.Message)
		}
	}
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	
	// Зберігаємо ID повідомлення користувача
	b.mu.Lock()
	if b.messageIDs[chatID] == nil {
		b.messageIDs[chatID] = make([]int, 0)
	}
	b.messageIDs[chatID] = append(b.messageIDs[chatID], message.MessageID)
	b.mu.Unlock()

	text := message.Text

	switch text {
	case "/start":
		b.handleStart(chatID)
	case "📊 Статистика":
		b.handleStats(chatID)
	case "⚙️ Налаштування":
		b.handleSettings(chatID)
	case "🔄 Новий чат":
		b.handleNewChat(chatID)
	case "❓ Допомога":
		b.handleHelp(chatID)
	case bypassCode:
		b.handleBypassCode(chatID)
	default:
		if len(text) > 3 && text[:3] == "sk-" {
			b.handleAPIKey(chatID, text)
		} else {
			// Перевіряємо ліміт запитів тільки для звернень до GPT
			if !b.checkRequestLimit(chatID) && text != bypassCode {
				b.sendMessage(chatID, "⚠️ Ви досягли ліміту запитів на сьогодні. Використайте кодову фразу для необмеженого доступу.")
				return
			}
			b.handleGPTRequest(chatID, text)
		}
	}
}

func (b *Bot) handleStart(chatID int64) {
	// Перевіряємо, чи є вже API ключ
	apiKey, err := b.storage.GetAPIKey(chatID)
	if err == nil && apiKey != "" {
		text := `👋 З поверненням! Радий вас знову бачити.
Можемо продовжити нашу розмову. Просто надішліть мені ваше повідомлення!

Використовуйте кнопки внизу для керування ботом.`
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyMarkup = createMainKeyboard()
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
		}
		return
	}

	// Якщо ключа немає, показуємо початкове повідомлення
	text := `👋 Вітаю! Я бот, створений @hesher116, який допоможе вам спілкуватися з ChatGPT напряму через ваш OpenAI API ключ.

Для початку роботи, будь ласка, надішліть свій OpenAI API ключ.
Якщо у вас його немає, отримайте на сайті: https://platform.openai.com/account/api-keys`

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = createMainKeyboard()
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
}

func (b *Bot) handleHelp(chatID int64) {
	text := `📌 Доступні команди:

/start - Почати роботу
/help - Показати це повідомлення
/change_key - Змінити API ключ

Просто надішліть повідомлення, і я передам його до ChatGPT!`

	b.sendMessage(chatID, text)
}

func (b *Bot) handleAPIKey(chatID int64, apiKey string) {
	if err := b.storage.SaveAPIKey(chatID, apiKey); err != nil {
		b.sendMessage(chatID, "❌ Помилка збереження ключа. Спробуйте ще раз.")
		return
	}

	msg := tgbotapi.NewMessage(chatID, "✅ API ключ успішно збережено! Тепер ви можете надсилати повідомлення.")
	msg.ReplyMarkup = createMainKeyboard()
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
}

func (b *Bot) handleGPTRequest(chatID int64, text string) {
	b.mu.Lock()
	gpt, exists := b.chatGPTs[chatID]
	if !exists {
		apiKey, err := b.storage.GetAPIKey(chatID)
		if err != nil {
			b.mu.Unlock()
			b.sendMessage(chatID, "❌ Будь ласка, спочатку надішліть свій API ключ.")
			return
		}
		model, _ := b.storage.GetUserSettings(chatID)
		gpt = api.NewChatGPT(apiKey)
		gpt.SetModel(model)
		b.chatGPTs[chatID] = gpt
	}
	b.mu.Unlock()

	response, err := gpt.SendMessage(text)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Помилка: %v", err))
		return
	}

	if err := b.storage.SaveToHistory(chatID, text, response); err != nil {
		log.Printf("Помилка збереження в історію: %v", err)
	}

	b.sendMessage(chatID, response, true)
}

func (b *Bot) sendMessage(chatID int64, text string, markdown ...bool) {
	msg := tgbotapi.NewMessage(chatID, text)
	if len(markdown) > 0 && markdown[0] {
		msg.ParseMode = tgbotapi.ModeMarkdown
	}

	sent, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return
	}

	// Зберігаємо ID повідомлення бота
	b.mu.Lock()
	if b.messageIDs[chatID] == nil {
		b.messageIDs[chatID] = make([]int, 0)
	}
	b.messageIDs[chatID] = append(b.messageIDs[chatID], sent.MessageID)
	b.mu.Unlock()
}

// Оновлюємо функцію createMainKeyboard
func createMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	buttons := [][]tgbotapi.KeyboardButton{
		{
			tgbotapi.KeyboardButton{Text: "📊 Статистика"},
			tgbotapi.KeyboardButton{Text: "⚙️ Налаштування"},
		},
		{
			tgbotapi.KeyboardButton{Text: "🔄 Новий чат"},
			tgbotapi.KeyboardButton{Text: "❓ Допомога"},
		},
	}

	return tgbotapi.ReplyKeyboardMarkup{
		Keyboard:       buttons,
		ResizeKeyboard: true,
	}
}

// Додаємо нові обробники
func (b *Bot) handleStats(chatID int64) {
	b.mu.RLock()
	stats := b.users[chatID]
	b.mu.RUnlock()

	if stats == nil {
		b.sendMessage(chatID, "📊 Статистика:\nЗапитів сьогодні: 0")
		return
	}

	text := fmt.Sprintf("📊 Статистика:\nЗапитів сьогодні: %d/%d", 
		stats.RequestCount, maxRequestsPerDay)
	b.sendMessage(chatID, text)
}

func (b *Bot) handleSettings(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("GPT-3.5", "model_gpt3"),
			tgbotapi.NewInlineKeyboardButtonData("GPT-4", "model_gpt4"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, "⚙️ Виберіть модель GPT:")
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
}

func (b *Bot) handleBypassCode(chatID int64) {
	b.mu.Lock()
	b.users[chatID] = &UserSettings{} // Скидаємо лічильник
	b.mu.Unlock()
	b.sendMessage(chatID, "✅ Ліміт запитів знято")
}

// Додаємо перевірку ліміту запитів
func (b *Bot) checkRequestLimit(chatID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.users[chatID] == nil {
		b.users[chatID] = &UserSettings{Model: api.ModelGPT3}
	}

	settings := b.users[chatID]

	if time.Since(settings.LastRequest).Hours() >= 24 {
		settings.RequestCount = 0
	}

	if settings.RequestCount >= maxRequestsPerDay {
		return false
	}

	settings.RequestCount++
	settings.LastRequest = time.Now()
	return true
}

// Оновлюємо обробник callback-ів
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	log.Printf("Отримано callback %s від користувача %d", callback.Data, chatID)

	callbackResponse := tgbotapi.NewCallback(callback.ID, "")
	if _, err := b.api.Request(callbackResponse); err != nil {
		log.Printf("Помилка відповіді на callback: %v", err)
	}

	var msg tgbotapi.MessageConfig // Оголошуємо змінну на початку функції

	switch callback.Data {
	case "clear_confirm":
		// Отримуємо останні повідомлення
		updates, err := b.api.GetUpdates(tgbotapi.UpdateConfig{
			Offset:  0,
			Limit:   100,
			Timeout: 0,
		})
		if err != nil {
			log.Printf("Помилка отримання повідомлень: %v", err)
			return
		}

		// Видаляємо кожне повідомлення
		for _, update := range updates {
			if update.Message != nil && update.Message.Chat.ID == chatID {
				deleteMsg := tgbotapi.NewDeleteMessage(chatID, update.Message.MessageID)
				if _, err := b.api.Request(deleteMsg); err != nil {
					log.Printf("Помилка видалення повідомлення %d: %v", update.Message.MessageID, err)
				}
			}
		}

		// Надсилаємо нове повідомлення про успішне очищення
		msg = tgbotapi.NewMessage(chatID, "✅ Історію чату очищено")
		msg.ReplyMarkup = createMainKeyboard()

	case "clear_cancel":
		msg = tgbotapi.NewMessage(chatID, "🔄 Очищення історії скасовано")

	case "model_gpt3":
		log.Printf("Змінюємо модель на GPT-3.5 для користувача %d", chatID)
		if err := b.storage.SaveUserSettings(chatID, api.ModelGPT3); err != nil {
			msg = tgbotapi.NewMessage(chatID, "❌ Помилка зміни моделі")
			log.Printf("Помилка збереження налаштувань: %v", err)
		} else {
			msg = tgbotapi.NewMessage(chatID, "✅ Модель змінено на GPT-3.5")
		}

	case "model_gpt4":
		log.Printf("Змінюємо модель на GPT-4 для користувача %d", chatID)
		if err := b.storage.SaveUserSettings(chatID, api.ModelGPT4); err != nil {
			msg = tgbotapi.NewMessage(chatID, "❌ Помилка зміни моделі")
			log.Printf("Помилка збереження налаштувань: %v", err)
		} else {
			msg = tgbotapi.NewMessage(chatID, "✅ Модель змінено на GPT-4")
		}
	}

	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
}

// Додаємо новий обробник для нового чату
func (b *Bot) handleNewChat(chatID int64) {
	// Отримуємо API ключ для перевірки
	apiKey, err := b.storage.GetAPIKey(chatID)
	if err != nil || apiKey == "" {
		text := `👋 Для початку роботи, будь ласка, надішліть свій OpenAI API ключ.
Якщо у вас його немає, отримайте на сайті: https://platform.openai.com/account/api-keys`
		b.sendMessage(chatID, text)
		return
	}

	// Показуємо індикатор процесу
	msg := tgbotapi.NewMessage(chatID, "🔄 Очищення чату...")
	statusMsg, _ := b.api.Send(msg)

	// Створюємо новий екземпляр ChatGPT заздалегідь
	b.mu.Lock()
	model, _ := b.storage.GetUserSettings(chatID)
	gpt := api.NewChatGPT(apiKey)
	gpt.SetModel(model)
	b.chatGPTs[chatID] = gpt
	b.mu.Unlock()

	// Готуємо текст нового повідомлення
	text := `🆕 Починаємо новий чат!

Я готовий допомогти вам з будь-якими питаннями. 
Просто напишіть ваше повідомлення.

Поточна модель: `

	if model == api.ModelGPT3 {
		text += "GPT-3.5"
	} else {
		text += "GPT-4"
	}

	// Видаляємо всі попередні повідомлення
	lastMsg := b.getLastMessageID(chatID)
	log.Printf("Починаємо видалення повідомлень для чату %d, останній ID: %d", chatID, lastMsg)
	
	// Використовуємо адаптивний крок
	deleteChan := make(chan struct{}, 50) // Збільшуємо паралельність
	
	// Розбиваємо діапазон на сегменти з різними кроками
	segments := []struct {
		start, end, step int
	}{
		{lastMsg - 100, lastMsg, 1},      // Останні 100 повідомлень видаляємо по одному
		{lastMsg - 500, lastMsg - 100, 5}, // Наступні 400 повідомлень видаляємо по 5
		{0, lastMsg - 500, 20},           // Старі повідомлення видаляємо по 20
	}

	for _, seg := range segments {
		for i := seg.end; i > seg.start && i > 0; i -= seg.step {
			deleteChan <- struct{}{}
			go func(msgID, step int) {
				defer func() { <-deleteChan }()
				
				// Видаляємо блок повідомлень
				for j := msgID; j > msgID-step && j > 0; j-- {
					if j != statusMsg.MessageID {
						deleteMsg := tgbotapi.NewDeleteMessage(chatID, j)
						if _, err := b.api.Request(deleteMsg); err == nil {
							log.Printf("Видалено повідомлення %d в чаті %d", j, chatID)
						}
					}
				}
			}(i, seg.step)
		}
	}

	// Чекаємо завершення всіх горутин
	for i := 0; i < cap(deleteChan); i++ {
		deleteChan <- struct{}{}
	}

	// Очищаємо історію в базі даних
	if err := b.storage.ClearHistory(chatID); err != nil {
		log.Printf("Помилка очищення історії: %v", err)
	}

	// Видаляємо індикатор процесу
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID)
	b.api.Request(deleteMsg)

	// В останню чергу надсилаємо нове повідомлення
	newMsg := tgbotapi.NewMessage(chatID, text)
	newMsg.ReplyMarkup = createMainKeyboard()
	sent, err := b.api.Send(newMsg)
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return
	}

	// Зберігаємо ID нового повідомлення
	b.mu.Lock()
	b.messageIDs[chatID] = []int{sent.MessageID}
	b.mu.Unlock()
}

// Оптимізуємо отримання останнього ID
func (b *Bot) getLastMessageID(chatID int64) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if messages, ok := b.messageIDs[chatID]; ok && len(messages) > 0 {
		return messages[len(messages)-1] + 10 // Зменшуємо запас, бо використовуємо адаптивний крок
	}
	return 50 // Зменшуємо початкову кількість спроб
} 