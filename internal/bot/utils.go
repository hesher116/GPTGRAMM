package bot

import (
	"log"
)

const (
	maxRequestsPerDay = 5
	bypassCode        = "1111"
	maxStoredMessages = 100
	logFormat         = "%-25s | %-10d | %s\n"
)

func logAction(action string, chatID int64, details string) {
	log.Printf(logFormat, action, chatID, details)
}
