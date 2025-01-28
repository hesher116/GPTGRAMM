package monitoring

import (
	"os"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func init() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&logrus.JSONFormatter{})
}

type LogEntry struct {
	UserID    int64
	Action    string
	Error     error
	ExtraData map[string]interface{}
}

func LogError(entry LogEntry) {
	fields := logrus.Fields{
		"user_id": entry.UserID,
		"action":  entry.Action,
	}

	if entry.Error != nil {
		fields["error"] = entry.Error.Error()
	}

	for k, v := range entry.ExtraData {
		fields[k] = v
	}

	log.WithFields(fields).Error("Помилка виконання операції")
}

func LogInfo(entry LogEntry) {
	fields := logrus.Fields{
		"user_id": entry.UserID,
		"action":  entry.Action,
	}

	for k, v := range entry.ExtraData {
		fields[k] = v
	}

	log.WithFields(fields).Info("Виконано операцію")
} 