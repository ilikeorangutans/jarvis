package jarvis

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func AddWeatherHandler(ctx context.Context, b *bot.Bot) {
	cityCode := "on-143"
	r := regexp.MustCompile(`(?i)\Aweather`)

	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			client.SendText(evt.RoomID, "Please wait while I fetch the forecast for you...")
			resp, err := http.Get(fmt.Sprintf("https://weather.gc.ca/rss/city/%s_e.xml", cityCode))
			if err != nil {
				client.SendText(evt.RoomID, fmt.Sprintf("I'm unable to retrieve the forecast. 😔 (%s)", err.Error()))
				return nil
			}
			defer resp.Body.Close()

			var feed Feed
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				client.SendText(evt.RoomID, fmt.Sprintf("I'm unable to retrieve the forecast. 😔 (%s)", err.Error()))
				return nil
			}
			// TODO error handle
			if err := xml.Unmarshal(data, &feed); err != nil {
				log.Error().Err(err).Msg("decoding xml")
			}

			client.SendText(evt.RoomID, formatFeed(feed))
			return nil
		},

		predicates.All(
			predicates.MessageMatching(r),
			predicates.NotFromUser(b.UserID),
		),
	)
}

func formatFeed(feed Feed) string {
	// TODO build html
	var builder strings.Builder
	builder.WriteString("Weather for ")
	builder.WriteString(feed.Title)
	builder.WriteString("\n")
	builder.WriteString(feed.Warnings())
	builder.WriteString("\n")
	builder.WriteString(feed.CurrentCondition())

	for i := 2; i < 6; i++ {
		builder.WriteString("\n")
		builder.WriteString(" - ")
		builder.WriteString(feed.Entries[i].Title)
	}

	return builder.String()
}

type Feed struct {
	Title   string  `xml:"title"`
	Entries []Entry `xml:"entry"`
}

func (f Feed) HasWarnings() bool {
	return !strings.HasPrefix(strings.ToLower(f.Entries[0].Title), "no watches or warnings")
}

func (f Feed) Warnings() string {
	return f.Entries[0].Summary
}

func (f Feed) CurrentCondition() string {
	return f.Entries[1].Title
}

type Entry struct {
	Title    string   `xml:"title"`
	Category Category `xml:"category"`
	Summary  string   `xml:"summary"`
}

type Category struct {
	Term string `xml:"term,attr"`
}
