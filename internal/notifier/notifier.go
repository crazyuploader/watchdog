package notifier

// Notifier defines the interface for sending notifications.
// This abstraction allows us to support multiple notification backends
// (webhook/Apprise, Telegram, email, etc.) with a consistent interface.
//
// Implementations of this interface should handle:
//   - Formatting the notification message appropriately for their backend
//   - Sending the notification via their specific protocol/API
//   - Handling errors and retries if necessary
type Notifier interface {
	// SendNotification sends a notification with the given subject and message.
	// Parameters:
	//   - subject: The notification title/subject (e.g., "Telnyx Balance Alert")
	//   - message: The notification body/details (e.g., "Balance is $5.00, below threshold")
	// Returns:
	//   - An error if the notification fails to send, nil on success
	SendNotification(subject, message string) error
}
