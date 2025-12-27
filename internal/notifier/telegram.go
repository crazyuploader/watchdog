package notifier

import (
	"fmt"
)

type TelegramNotifier struct {
	BotToken string
	ChatID   string
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{BotToken: botToken, ChatID: chatID}
}

func (t *TelegramNotifier) SendNotification(subject, message string) error {
	// Mock implementation for Telegram notification
	fullMessage := fmt.Sprintf("%s\n\n%s", subject, message)
	fmt.Printf("Sending Telegram notification to chat ID %s: %s\n", t.ChatID, fullMessage)
	return nil
}
