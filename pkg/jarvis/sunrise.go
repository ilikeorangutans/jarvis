package jarvis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/nathan-osman/go-sunrise"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

type geolocationResp struct {
	CountryName string  `json:"country_name"`
	City        string  `json:"city"`
	TimeZone    string  `json:"time_zone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

func AddSunriseHandlers(ctx context.Context, b *bot.Bot) error {
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {

			url := "https://freegeoip.app/json/"

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return fmt.Errorf("could not create geolocation request: %w", err)
			}
			req.Header.Add("accept", "application/json")
			req.Header.Add("content-type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("could not make geolocation request: %w", err)
			}
			defer res.Body.Close()
			decoder := json.NewDecoder(res.Body)
			var geolocation geolocationResp
			err = decoder.Decode(&geolocation)
			if err != nil {
				return fmt.Errorf("error decoding json: %w", err)
			}

			location, err := time.LoadLocation(geolocation.TimeZone)
			if err != nil {
				return fmt.Errorf("could not load location: %w", err)
			}
			now := time.Now()
			rise, set := sunrise.SunriseSunset(
				geolocation.Latitude, geolocation.Longitude,
				now.Year(), now.Local().Month(), now.Day(),
			)

			client.SendHTML(evt.RoomID, fmt.Sprintf("🌄 sunrise at %s, 🌇 sunset at %s", rise.In(location).Format("15:04"), set.In(location).Format("15:04")))
			return nil
		},
		predicates.All(
			predicates.MessageMatching(regexp.MustCompile(`(?i)\A\s*sunrise|sunset`)),
		),
	)

	return nil

}
