package domain

type CalendarEventMapping struct {
	AppointmentID int64
	GoogleEventID string
	Status        SyncStatus
}
