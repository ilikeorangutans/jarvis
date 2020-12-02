package jarvis

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func AddDiceHandler(b *bot.Bot) {
	r := regexp.MustCompile(`(?i)\A([0-9]+\s+)?dice\s+roll\s*`)
	b.On(func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
		msg := evt.Content.AsMessage()

		parts := r.FindStringSubmatch(msg.Body)
		diceRoll, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			diceRoll = 1
		}
		if diceRoll > 20 {
			client.SendText(evt.RoomID, fmt.Sprintf("Sorry, I don't have that many dice!"))
			return nil
		}

		var rolls []string
		b := make([]byte, diceRoll)
		rand.Read(b)
		for i := 0; i < diceRoll; i++ {
			rolls = append(rolls, fmt.Sprintf("%d", (b[i]%6+1)))
		}
		client.SendText(evt.RoomID, fmt.Sprintf("🎲 I've rolled the dice for you: %s", strings.Join(rolls, ", ")))
		return nil
	}, predicates.MessageMatching(r))
}
