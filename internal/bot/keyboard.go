package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
		{
			tgbotapi.NewKeyboardButton("🌞 Погода"),
		},
	}

	return tgbotapi.ReplyKeyboardMarkup{
		Keyboard:       buttons,
		ResizeKeyboard: true,
	}
}
