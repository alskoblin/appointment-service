package domain

type NotificationStatus string

const (
	NotificationStatusSent    NotificationStatus = "sent"
	NotificationStatusFailed  NotificationStatus = "failed"
	NotificationStatusSkipped NotificationStatus = "skipped"
)
