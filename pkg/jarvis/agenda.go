package jarvis

import (
	"context"
	"regexp"
	"strings"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func AddAgendaHandlers(ctx context.Context, b *bot.Bot) error {
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			msg := evt.Content.AsMessage()
			if strings.Contains(strings.ToLower(msg.Body), "enable") {
				client.SendText(evt.RoomID, "agenda enable")
			} else if strings.Contains(strings.ToLower(msg.Body), "disable") {
				client.SendText(evt.RoomID, "agenda disable")
			} else {
				client.SendText(evt.RoomID, "agenda status")
			}
			return nil
		},
		predicates.All(
			predicates.MessageMatching(regexp.MustCompile(`(?i)\A\s*agenda\s+(enable|disable|status)`)),
		),
	)

	return nil
}
