package bot

import (
	"GPTGRAMM/internal/api"
	"GPTGRAMM/internal/storage"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Додаємо нові типи та константи
const (
	maxRequestsPerDay = 10
	bypassCode        = "UNLIMITED_ACCESS" // Кодова фраза для обходу ліміту
	maxStoredMessages = 100                // Максимальна кількість збережених ID повідомлень
	logFormat         = "%-25s | %-10d | %s\n"
)

// Додаємо структуру для налаштувань користувача
type UserSettings struct {
	Model        string
	RequestCount int
	LastRequest  time.Time
}

// Bot представляє основну структуру телеграм бота
type Bot struct {
	api        *tgbotapi.BotAPI
	storage    *storage.Storage
	chatGPTs   sync.Map
	users      sync.Map
	messageIDs sync.Map // chatID -> *MessageQueue
}

// MessageQueue зберігає обмежену кількість ID повідомлень
type MessageQueue struct {
	mu      sync.Mutex
	ids     []int
	maxSize int
}

func NewMessageQueue(size int) *MessageQueue {
	return &MessageQueue{
		ids:     make([]int, 0, size),
		maxSize: size,
	}
}

func (q *MessageQueue) Add(id int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ids = append(q.ids, id)
	if len(q.ids) > q.maxSize {
		q.ids = q.ids[1:]
	}
}

func (q *MessageQueue) GetAll() []int {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]int, len(q.ids))
	copy(result, q.ids)
	return result
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
		if update.CallbackQuery != nil {
			go b.handleCallback(update.CallbackQuery)
		} else if update.Message != nil {
			go b.handleMessage(update.Message)
		}
	}
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	text := message.Text

	// Зберігаємо ID повідомлення
	queue, _ := b.messageIDs.LoadOrStore(chatID, NewMessageQueue(maxStoredMessages))
	queue.(*MessageQueue).Add(message.MessageID)

	switch text {
	case "/start":
		logAction("КОМАНДА", chatID, "👋 Початок роботи")
		b.handleStart(chatID)
	case "📊 Статистика":
		logAction("КОМАНДА", chatID, "📊 Перегляд статистики")
		b.handleStats(chatID)
	case "⚙️ Налаштування":
		logAction("КОМАНДА", chatID, "⚙️ Відкрито налаштування")
		b.handleSettings(chatID)
	case "🔄 Новий чат":
		b.handleNewChat(chatID)
	case "❓ Допомога":
		logAction("КОМАНДА", chatID, "❓ Запит допомоги")
		b.handleHelp(chatID)
	case bypassCode:
		logAction("КОМАНДА", chatID, "🔓 Використано код обходу ліміту")
		b.handleBypassCode(chatID)
	default:
		if len(text) > 3 && text[:3] == "sk-" {
			logAction("КОМАНДА", chatID, "🔑 Отримано API ключ")
			b.handleAPIKey(chatID, text)
		} else {
			// Перевіряємо ліміт запитів тільки для звернень до GPT
			if !b.checkRequestLimit(chatID) && text != bypassCode {
				logAction("ПОМИЛКА", chatID, "⚠️ Досягнуто ліміт запитів")
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
	logAction("API_KEY", chatID, "Спроба зберегти новий API ключ")

	if err := b.storage.SaveAPIKey(chatID, apiKey); err != nil {
		logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося зберегти API ключ: %v", err))
		b.sendMessage(chatID, "❌ Помилка збереження ключа. Спробуйте ще раз.")
		return
	}

	logAction("API_KEY", chatID, "API ключ успішно збережено")
	msg := tgbotapi.NewMessage(chatID, "✅ API ключ успішно збережено! Тепер ви можете надсилати повідомлення.")
	msg.ReplyMarkup = createMainKeyboard()
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
}

func (b *Bot) handleGPTRequest(chatID int64, text string) {
	// Отримуємо існуючий або створюємо новий ChatGPT
	gptInstance, _ := b.chatGPTs.LoadOrStore(chatID, func() interface{} {
		apiKey, err := b.storage.GetAPIKey(chatID)
		if err != nil {
			return nil
		}
		model, _ := b.storage.GetUserSettings(chatID)
		gpt := api.NewChatGPT(apiKey)
		gpt.SetModel(model)
		return gpt
	}())

	if gptInstance == nil {
		b.sendMessage(chatID, "❌ Будь ласка, спочатку надішліть свій API ключ.")
		return
	}

	gpt := gptInstance.(*api.ChatGPT)

	// Логуємо запит користувача
	model, _ := b.storage.GetUserSettings(chatID)
	modelName := map[string]string{api.ModelGPT3: "GPT-3.5", api.ModelGPT4: "GPT-4"}[model]
	logAction("ДІАЛОГ", chatID, fmt.Sprintf("\nЗапит (%s):\n%s", modelName, text))

	response, err := gpt.SendMessage(text)
	if err != nil {
		logAction("ПОМИЛКА", chatID, fmt.Sprintf("Помилка GPT: %v", err))
		b.sendMessage(chatID, fmt.Sprintf("❌ Помилка: %v", err))
		return
	}

	// Логуємо відповідь GPT
	logAction("ДІАЛОГ", chatID, fmt.Sprintf("\nВідповідь:\n%s", response))

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

	// Завжди додаємо основну клавіатуру, якщо не вказано інше
	if msg.ReplyMarkup == nil {
		msg.ReplyMarkup = createMainKeyboard()
	}

	sent, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return
	}

	// Зберігаємо ID повідомлення бота
	queue, _ := b.messageIDs.LoadOrStore(chatID, NewMessageQueue(maxStoredMessages))
	queue.(*MessageQueue).Add(sent.MessageID)
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
	value, exists := b.users.Load(chatID)

	var requestCount int
	if exists && value != nil {
		settings := value.(UserSettings)
		requestCount = settings.RequestCount
	}

	text := fmt.Sprintf("📊 Статистика:\nЗапитів сьогодні: %d/%d",
		requestCount, maxRequestsPerDay)
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
	// Отримуємо поточні налаштування або створюємо нові
	value, _ := b.users.Load(chatID)
	var settings UserSettings
	if value != nil {
		settings = value.(UserSettings)
	}

	// Скидаємо лічильник, зберігаючи інші налаштування
	settings.RequestCount = 0
	settings.LastRequest = time.Now()

	b.users.Store(chatID, settings)
	b.sendMessage(chatID, "✅ Ліміт запитів знято")
}

// Додаємо перевірку ліміту запитів
func (b *Bot) checkRequestLimit(chatID int64) bool {
	value, exists := b.users.Load(chatID)

	var settings UserSettings
	if !exists || value == nil {
		// Створюємо нові налаштування для нового користувача
		settings = UserSettings{
			Model:        api.ModelGPT3,
			RequestCount: 0,
			LastRequest:  time.Now(),
		}
	} else {
		settings = value.(UserSettings)
	}

	// Скидаємо лічильник, якщо минуло 24 години
	if time.Since(settings.LastRequest).Hours() >= 24 {
		settings.RequestCount = 0
		settings.LastRequest = time.Now()
	}

	// Перевіряємо ліміт
	if settings.RequestCount >= maxRequestsPerDay {
		return false
	}

	// Оновлюємо лічильник
	settings.RequestCount++
	settings.LastRequest = time.Now()
	b.users.Store(chatID, settings)

	return true
}

// Оновлюємо обробник callback-ів
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	logAction("CALLBACK", chatID, fmt.Sprintf("Дія: %s", callback.Data))

	callbackResponse := tgbotapi.NewCallback(callback.ID, "")
	if _, err := b.api.Request(callbackResponse); err != nil {
		log.Printf("Помилка відповіді на callback: %v", err)
	}

	var msg tgbotapi.MessageConfig

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
		// Отримуємо поточну модель
		currentModel, _ := b.storage.GetUserSettings(chatID)
		if currentModel == api.ModelGPT3 {
			logAction("МОДЕЛЬ", chatID, "Спроба зміни на поточну модель GPT-3.5")
			msg = tgbotapi.NewMessage(chatID, "ℹ️ GPT-3.5 вже є поточною моделлю")
		} else {
			logAction("МОДЕЛЬ", chatID, "Зміна на GPT-3.5")
			if err := b.storage.SaveUserSettings(chatID, api.ModelGPT3); err != nil {
				logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося змінити модель: %v", err))
				msg = tgbotapi.NewMessage(chatID, "❌ Помилка зміни моделі")
			} else {
				msg = tgbotapi.NewMessage(chatID, "✅ Модель змінено на GPT-3.5")
			}
		}

	case "model_gpt4":
		// Отримуємо поточну модель
		currentModel, _ := b.storage.GetUserSettings(chatID)
		if currentModel == api.ModelGPT4 {
			logAction("МОДЕЛЬ", chatID, "Спроба зміни на поточну модель GPT-4")
			msg = tgbotapi.NewMessage(chatID, "ℹ️ GPT-4 вже є поточною моделлю")
		} else {
			logAction("МОДЕЛЬ", chatID, "Зміна на GPT-4")
			if err := b.storage.SaveUserSettings(chatID, api.ModelGPT4); err != nil {
				logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося змінити модель: %v", err))
				msg = tgbotapi.NewMessage(chatID, "❌ Помилка зміни моделі")
			} else {
				msg = tgbotapi.NewMessage(chatID, "✅ Модель змінено на GPT-4")
			}
		}
	}

	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
	}
}

// Оновлюємо новий обробник для нового чату
func (b *Bot) handleNewChat(chatID int64) {
	logAction("КОМАНДА", chatID, "🔄 Новий чат")

	// Перевірка API ключа
	apiKey, err := b.storage.GetAPIKey(chatID)
	if err != nil || apiKey == "" {
		logAction("ПОМИЛКА", chatID, "API ключ не знайдено")
		b.sendMessage(chatID, "👋 Для початку роботи, будь ласка, надішліть свій OpenAI API ключ.")
		return
	}

	// Отримуємо поточну модель
	model, _ := b.storage.GetUserSettings(chatID)

	// Спочатку надсилаємо повідомлення про початок нового чату
	text := fmt.Sprintf(`🆕 Починаємо новий чат!

🤖 Поточна модель: %s
⚡️ Підготовка до очищення...

💡 Будь ласка, зачекайте кілька секунд`,
		map[string]string{api.ModelGPT3: "GPT-3.5", api.ModelGPT4: "GPT-4"}[model])

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = createMainKeyboard()
	tempMsg, err := b.api.Send(msg)
	if err != nil {
		logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося надіслати повідомлення: %v", err))
		return
	}

	// Очищаємо історію в базі даних
	_ = b.storage.ClearHistory(chatID)

	// Процес видалення повідомлень
	deletedCount := 0
	failedCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Беремо більший діапазон для надійності
	lastMsgID := tempMsg.MessageID
	startID := lastMsgID - 1000
	if startID < 1 {
		startID = 1
	}

	workers := 20
	messagesChan := make(chan int, lastMsgID-startID)

	// Запускаємо воркери
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for msgID := range messagesChan {
				if msgID != tempMsg.MessageID { // Не видаляємо наше тимчасове повідомлення
					deleteMsg := tgbotapi.NewDeleteMessage(chatID, msgID)
					if _, err := b.api.Request(deleteMsg); err != nil {
						if !strings.Contains(err.Error(), "message to delete not found") {
							mu.Lock()
							failedCount++
							mu.Unlock()
						}
					} else {
						mu.Lock()
						deletedCount++
						mu.Unlock()
					}
				}
			}
		}()
	}

	// Відправляємо ID на видалення
	for msgID := lastMsgID; msgID > startID; msgID-- {
		messagesChan <- msgID
	}
	close(messagesChan)
	wg.Wait()

	// Оновлюємо стан чату
	gpt := api.NewChatGPT(apiKey)
	gpt.SetModel(model)
	b.chatGPTs.Store(chatID, gpt)

	// Видаляємо тимчасове повідомлення
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, tempMsg.MessageID)
	b.api.Request(deleteMsg)

	// Надсилаємо нове повідомлення з результатами
	resultText := fmt.Sprintf(`🆕 Починаємо новий чат!

🤖 Поточна модель: %s
🗑️ Видалено повідомлень: %d

💭 Можете продовжувати спілкування з чатом або вибрати наступну дію кнопками нижче`,
		map[string]string{api.ModelGPT3: "GPT-3.5", api.ModelGPT4: "GPT-4"}[model],
		deletedCount)

	finalMsg := tgbotapi.NewMessage(chatID, resultText)
	finalMsg.ReplyMarkup = createMainKeyboard()
	sent, err := b.api.Send(finalMsg)
	if err != nil {
		logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося надіслати фінальне повідомлення: %v", err))
		return
	}

	// Зберігаємо ID нового повідомлення
	queue, _ := b.messageIDs.LoadOrStore(chatID, NewMessageQueue(maxStoredMessages))
	queue.(*MessageQueue).Add(sent.MessageID)

	logAction("РЕЗУЛЬТАТ", chatID, fmt.Sprintf("Видалено повідомлень: %d", deletedCount))
}

// Оновлюємо функцію logAction
func logAction(action string, chatID int64, details string) {

	log.Printf(logFormat, action, chatID, details)
}
