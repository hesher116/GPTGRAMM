package bot

import (
	"fmt"
	"sync"
	"sync/atomic"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type MessageTools struct {
	api *tgbotapi.BotAPI
}

func NewMessageTools(api *tgbotapi.BotAPI) *MessageTools {
	return &MessageTools{
		api: api,
	}
}

func (mt *MessageTools) DeleteMessages(chatID int64, lastMsgID int) (int, error) {
	var deletedCount int32
	var wg sync.WaitGroup

	// Оптимізація діапазону
	startID := lastMsgID - 50
	if startID < 1 {
		startID = 1
	}

	deleteMessages := make([]tgbotapi.DeleteMessageConfig, 0, lastMsgID-startID)
	for msgID := lastMsgID; msgID > startID; msgID-- {
		deleteMessages = append(deleteMessages, tgbotapi.NewDeleteMessage(chatID, msgID))
	}

	workers := min(10, len(deleteMessages))

	msgChan := make(chan tgbotapi.DeleteMessageConfig, len(deleteMessages))

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for msg := range msgChan {
				if _, err := mt.api.Request(msg); err == nil {
					atomic.AddInt32(&deletedCount, 1)
				}
			}
		}()
	}

	for _, msg := range deleteMessages {
		msgChan <- msg
	}
	close(msgChan)

	wg.Wait()

	if deletedCount == 0 {
		logAction("ВИДАЛЕННЯ", chatID, "Не вдалося видалити жодне повідомлення")
	} else {
		logAction("ВИДАЛЕННЯ", chatID, fmt.Sprintf("Видалено %d повідомлень", deletedCount))
	}

	return int(deletedCount), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
