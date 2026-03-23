package domain

import "time"

type Specialist struct {
	ID                  int64     `json:"id"`
	FullName            string    `json:"full_name"`
	Profession          string    `json:"profession"`
	SlotDurationMinutes int       `json:"slot_duration_minutes"`
	Timezone            string    `json:"timezone"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}
