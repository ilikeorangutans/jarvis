package jarvis

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/nathan-osman/go-sunrise"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func AddSunriseHandlers(ctx context.Context, b *bot.Bot) error {
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			rise, set := sunrise.SunriseSunset(
				43.65, -79.38, // Toronto, CA
				2000, time.January, 1, // 2000-01-01
			)

			client.SendHTML(evt.RoomID, fmt.Sprintf("🌄 sunrise at %s, 🌇 sunset at %s", rise.Local().Format("15:04"), set.Local().Format("15:04")))
			return nil
		},
		predicates.All(
			predicates.MessageMatching(regexp.MustCompile(`(?i)\A\s*sunrise|sunset`)),
		),
	)

	return nil

}
