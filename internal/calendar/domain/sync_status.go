package domain

type SyncStatus string

const (
	SyncStatusSynced  SyncStatus = "synced"
	SyncStatusDeleted SyncStatus = "deleted"
	SyncStatusFailed  SyncStatus = "failed"
	SyncStatusSkipped SyncStatus = "skipped"
)
