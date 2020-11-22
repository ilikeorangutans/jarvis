package bot

import (
	"time"

	"maunium.net/go/mautrix/id"
)

type Reminder struct {
	EventID id.EventID
	Message string
	// When is when the user should be reminded
	When time.Time
	User id.UserID
	Room id.RoomID
	// Timestamp is when the reminder was set
	Timestamp time.Time
}
