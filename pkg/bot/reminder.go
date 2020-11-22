package bot

import (
	"time"

	"maunium.net/go/mautrix/id"
)

type Reminder struct {
	EventID id.EventID
	Message string
	When    time.Time
	User    id.UserID
	Room    id.RoomID
}
