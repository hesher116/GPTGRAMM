package bot

import (
	"GPTGRAMM/internal/api"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	text := message.Text

	queue := b.getMessageQueue(chatID)
	queue.Add(message.MessageID)

	if state, ok := b.users.Load(chatID); ok && state == "awaiting_city" {
		b.getWeatherForCity(chatID, text)
		b.users.Delete(chatID)
		return
	}

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
	case "🌞 Погода":
		logAction("КОМАНДА", chatID, "🌞 Запит кастомної погоди")
		b.customWeather(chatID)
	case bypassCode:
		logAction("КОМАНДА", chatID, "🔓 Використано код обходу ліміту")
		b.handleBypassCode(chatID)
	default:
		if len(text) > 3 && text[:3] == "sk-" {
			logAction("КОМАНДА", chatID, "🔑 Отримано API ключ")
			b.handleAPIKey(chatID, text)
		} else {
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
	apiKey, err := b.Storage.GetAPIKey(chatID)
	if err == nil && apiKey != "" {
		text := `👋 З поверненням! Радий вас знову бачити.
Можемо продовжити нашу розмову. Просто надішліть мені ваше повідомлення!

Використовуйте кнопки внизу для керування ботом.`
		b.sendMessage(chatID, text)
		return
	}

	text := `👋 Вітаю! Я бот, створений @hesher116, який допоможе вам спілкуватися з ChatGPT напряму через ваш OpenAI API ключ.

Для початку роботи, будь ласка, надішліть свій OpenAI API ключ.
Якщо у вас його немає, отримайте на сайті: https://platform.openai.com/account/api-keys`
	b.sendMessage(chatID, text)
}

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

func (b *Bot) handleNewChat(chatID int64) {
	logAction("КОМАНДА", chatID, "🔄 Новий чат")

	apiKey, err := b.Storage.GetAPIKey(chatID)
	if err != nil || apiKey == "" {
		logAction("ПОМИЛКА", chatID, "API ключ не знайдено")
		b.sendMessage(chatID, "👋 Для початку роботи, будь ласка, надішліть свій OpenAI API ключ.")
		return
	}

	model, _ := b.Storage.GetUserSettings(chatID)
	if model == "" {
		model = api.ModelGPT3
		if err := b.Storage.SaveUserSettings(chatID, model); err != nil {
			log.Printf("Помилка збереження налаштувань моделі: %v", err)
		}
	}
	modelName := map[string]string{api.ModelGPT3: "GPT-3.5", api.ModelGPT4: "GPT-4"}[model]

	if err := b.Storage.ClearHistory(chatID); err != nil {
		log.Printf("Помилка очищення історії: %v", err)
		b.sendMessage(chatID, "⚠️ Помилка очищення історії. Будь ласка, спробуйте ще раз.")
	}

	time.Sleep(100 * time.Millisecond)
	gpt := api.NewChatGPT(apiKey)
	gpt.SetModel(model)
	b.chatGPTs.Store(chatID, gpt)

	text := fmt.Sprintf(`🆕 Починаємо новий чат!

🤖 Поточна модель: %s
⚡️ Очищення повідомлень...`, modelName)

	tempMsg, err := b.api.Send(tgbotapi.NewMessage(chatID, text))
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return
	}

	messageTools := NewMessageTools(b.api)
	deletedCount, _ := messageTools.DeleteMessages(chatID, tempMsg.MessageID)

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, tempMsg.MessageID)
	b.api.Request(deleteMsg)

	finalText := fmt.Sprintf(`🆕 Новий чат готовий!

🤖 Поточна модель: %s
🗑️ Видалено повідомлень: %d

💭 Можете починати спілкування`, modelName, deletedCount)
	b.sendMessage(chatID, finalText)
}

func (b *Bot) handleHelp(chatID int64) {
	text := `📌 Доступні команди:

/start - Почати роботу

Просто надішліть повідомлення, і я передам його до ChatGPT!`
	b.sendMessage(chatID, text)
}

func (b *Bot) customWeather(chatID int64) {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Київ"),
			tgbotapi.NewKeyboardButton("Кам'янець-Подільський"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Валенсія"),
			tgbotapi.NewKeyboardButton("Дананг"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "Виберіть місто або введіть назву міста, в якому хочете взнати погоду:")
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)

	b.users.Store(chatID, "awaiting_city")
}

func (b *Bot) getWeatherForCity(chatID int64, city string) {
	query := fmt.Sprintf("погода в %s. Відповідай українською.", city)
	b.sendMessage(chatID, fmt.Sprintf("Запитую погоду в місті %s...", city))

	gpt, err := b.getOrCreateGPTInstance(chatID)
	if err != nil {
		b.sendMessage(chatID, "❌ Будь ласка, спочатку надішліть свій API ключ.")
		return
	}

	response, err := gpt.SendMessage(query)
	if err != nil {
		log.Printf("Помилка запиту погоди для міста %s: %v", city, err)
		b.sendMessage(chatID, "Виникла помилка при запиті погоди. Спробуйте ще раз пізніше.")
		return
	}

	shortResponse := response
	if len(shortResponse) > 100 {
		shortResponse = shortResponse[:97] + "..."
	}
	logAction("ВІДПОВІДЬ", chatID, shortResponse)

	b.sendMessage(chatID, response)
}

func (b *Bot) handleBypassCode(chatID int64) {
	value, _ := b.users.Load(chatID)
	var settings UserSettings
	if value != nil {
		settings = value.(UserSettings)
	}

	settings.RequestCount = 0
	settings.LastRequest = time.Now()

	b.users.Store(chatID, settings)
	b.sendMessage(chatID, "✅ Ліміт запитів знято")
}

func (b *Bot) handleAPIKey(chatID int64, apiKey string) {
	if err := b.Storage.SaveAPIKey(chatID, apiKey); err != nil {
		logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося зберегти API ключ: %v", err))
		b.sendMessage(chatID, "❌ Помилка збереження ключа. Спробуйте ще раз.")
		return
	}

	logAction("API_KEY", chatID, "API ключ успішно збережено")
	b.sendMessage(chatID, "✅ API ключ успішно збережено! Тепер ви можете надсилати повідомлення.")
}

func (b *Bot) checkRequestLimit(chatID int64) bool {
	value, exists := b.users.Load(chatID)

	var settings UserSettings
	if !exists || value == nil {
		settings = UserSettings{
			Model:        api.ModelGPT3,
			RequestCount: 0,
			LastRequest:  time.Now(),
		}
	} else {
		settings = value.(UserSettings)
	}

	if time.Since(settings.LastRequest).Hours() >= 24 {
		settings.RequestCount = 0
		settings.LastRequest = time.Now()
	}

	if settings.RequestCount >= maxRequestsPerDay {
		return false
	}

	settings.RequestCount++
	settings.LastRequest = time.Now()
	b.users.Store(chatID, settings)

	return true
}

func (b *Bot) getOrCreateGPTInstance(chatID int64) (*api.ChatGPT, error) {
	gptInstance, _ := b.chatGPTs.LoadOrStore(chatID, func() interface{} {
		apiKey, err := b.Storage.GetAPIKey(chatID)
		if err != nil {
			return nil
		}
		model, _ := b.Storage.GetUserSettings(chatID)
		if model == "" {
			model = api.ModelGPT3
			if err := b.Storage.SaveUserSettings(chatID, model); err != nil {
				log.Printf("Помилка збереження налаштувань моделі: %v", err)
			}
		}
		gpt := api.NewChatGPT(apiKey)
		gpt.SetModel(model)
		return gpt
	}())

	if gptInstance == nil {
		return nil, fmt.Errorf("API ключ не знайдено")
	}

	return gptInstance.(*api.ChatGPT), nil
}

func (b *Bot) handleGPTRequest(chatID int64, text string) {
	gpt, err := b.getOrCreateGPTInstance(chatID)
	if err != nil {
		b.sendMessage(chatID, "❌ Будь ласка, спочатку надішліть свій API ключ.")
		return
	}

	currentModel, _ := b.Storage.GetUserSettings(chatID)
	if currentModel != "" && gpt.GetModel() != currentModel {
		gpt.SetModel(currentModel)
		gpt.ClearContext()
	}

	model, _ := b.Storage.GetUserSettings(chatID)
	modelName := map[string]string{api.ModelGPT3: "GPT-3.5", api.ModelGPT4: "GPT-4"}[model]
	logAction("ЗАПИТ", chatID, fmt.Sprintf("[%s] %s", modelName, text))

	response, err := gpt.SendMessage(text)
	if err != nil {
		logAction("ПОМИЛКА", chatID, fmt.Sprintf("Помилка GPT: %v", err))
		b.sendMessage(chatID, fmt.Sprintf("❌ Помилка: %v", err))
		return
	}

	shortResponse := response
	if len(shortResponse) > 100 {
		shortResponse = shortResponse[:97] + "..."
	}
	logAction("ВІДПОВІДЬ", chatID, shortResponse)

	if err := b.Storage.SaveToHistory(chatID, text, response); err != nil {
		log.Printf("Помилка збереження в історію: %v", err)
	}

	b.sendMessage(chatID, response, true)
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	logAction("CALLBACK", chatID, fmt.Sprintf("Дія: %s", callback.Data))

	callbackResponse := tgbotapi.NewCallback(callback.ID, "")
	if _, err := b.api.Request(callbackResponse); err != nil {
		log.Printf("Помилка відповіді на callback: %v", err)
	}

	// Визначаємо карти відповідностей на рівні пакету
	var (
		modelMap = map[string]string{
			"gpt3": api.ModelGPT3,
			"gpt4": api.ModelGPT4,
		}
		readableModelMap = map[string]string{
			"gpt3": "GPT-3.5",
			"gpt4": "GPT-4",
		}
	)

	switch callback.Data {
	case "model_gpt3", "model_gpt4":
		parts := strings.Split(callback.Data, "_")
		if len(parts) < 2 {
			logAction("ПОМИЛКА", chatID, "Некоректний формат callback.Data")
			return
		}
		currentModel := parts[1]

		// Перевіряємо, чи є така модель
		fullModelName, exists := modelMap[currentModel]
		if !exists {
			logAction("ПОМИЛКА", chatID, fmt.Sprintf("Невідома модель: %s", currentModel))
			return
		}

		// Отримуємо поточну модель користувача
		oldModel, err := b.Storage.GetUserSettings(chatID)
		logAction("НАЛАШТУВАННЯ", chatID, fmt.Sprintf("Поточна модель: %s, Нова модель: %s", oldModel, fullModelName))

		// Якщо модель вже встановлена
		if err == nil && oldModel == fullModelName {
			logAction("МОДЕЛЬ", chatID, fmt.Sprintf("Спроба зміни на поточну модель (%s)", fullModelName))
			b.sendMessage(chatID, fmt.Sprintf("ℹ️ %s вже є поточною моделлю", readableModelMap[currentModel]))
			return
		}

		// Збереження нової моделі
		if err := b.Storage.SaveUserSettings(chatID, fullModelName); err != nil {
			logAction("ПОМИЛКА", chatID, fmt.Sprintf("Не вдалося змінити модель: %v", err))
			b.sendMessage(chatID, "❌ Помилка зміни моделі")
			return
		}

		// Оновлення GPT-екземпляра
		if gptInstance, exists := b.chatGPTs.Load(chatID); exists {
			if gpt, ok := gptInstance.(*api.ChatGPT); ok {
				gpt.SetModel(fullModelName)
				gpt.ClearContext()
				b.chatGPTs.Store(chatID, gpt)
			}
		}

		// Логування і повідомлення користувачу
		logAction("МОДЕЛЬ", chatID, fmt.Sprintf("Зміна на %s", readableModelMap[currentModel]))
		b.sendMessage(chatID, fmt.Sprintf("✅ Модель змінено на %s", readableModelMap[currentModel]))
	}
}
