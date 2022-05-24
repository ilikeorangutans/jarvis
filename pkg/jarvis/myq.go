package jarvis

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/joeshaw/myq"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

var (
	authorizedUsers = map[string]struct{}{
		"@jakob:matrix.ilikeorangutans.me":  {},
		"@hannah:matrix.ilikeorangutans.me": {},
	}
)

func AddMyqHandler(b *bot.Bot) {
	b.On(func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
		msg := evt.Content.AsMessage()
		log.Info().Msgf("got message %q from %q", msg.Body, evt.Sender.String())

		if _, authorized := authorizedUsers[evt.Sender.String()]; !authorized {
			client.SendText(evt.RoomID, "I'm sorry, but you are not authorized to operate the garage door.")
			return nil
		}

		session := &myq.Session{
			Username: "26muir@gmail.com",
			Password: "@26Muir2021*",
		}

		garageDoorID := "CG0837E7EAA7"

		if strings.Contains(strings.ToLower(msg.Body), "open") {

		} else if strings.Contains(strings.ToLower(msg.Body), "open") {
			state, err := session.DeviceState(garageDoorID)
			if err != nil {
				return fmt.Errorf("getting device state: %w", err)
			}
			if state != myq.StateClosed {
				client.SendText(evt.RoomID, fmt.Sprintf("Cannot open garage door, it's %s", state))
			}

			err = session.SetDoorState(garageDoorID, myq.ActionOpen)
			if err != nil {
				client.SendText(evt.RoomID, fmt.Sprintf("An error occured while opening the garage door: %s", err))
				return fmt.Errorf("opening garage door: %w", err)
			}

			go func(ctx context.Context) {
				ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()

				for {
					select {
					case <-time.After(5 * time.Second):
					case <-ctx.Done():
					}
				}
			}(ctx)
		} else if strings.Contains(strings.ToLower(msg.Body), "devices") {
			client.SendText(evt.RoomID, "finding available devices...")
			devices, err := session.Devices()
			if err != nil {
				return fmt.Errorf("listing devices: %w", err)
			}

			for _, d := range devices {
				client.SendText(evt.RoomID, fmt.Sprintf("%s (%s, %s): %s", d.Name, d.Type, d.SerialNumber, d.DoorState))
			}
		} else {
			client.SendText(evt.RoomID, "Checking on the garage door for you...")
			state, err := session.DeviceState(garageDoorID)
			if err != nil {
				return fmt.Errorf("getting device state: %w", err)
			}
			client.SendText(evt.RoomID, fmt.Sprintf("The garage door is %s.", state))
		}

		return nil
	},
		predicates.MessageMatching(regexp.MustCompile(`(?i)garage.*(door)?`)),
	)
}
