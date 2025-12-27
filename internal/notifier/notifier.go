package notifier

type Notifier interface {
	SendNotification(subject, message string) error
}
