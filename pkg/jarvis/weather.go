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
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func AddWeatherHandler(ctx context.Context, b *bot.Bot) {
	cityCode := "on-143"
	r := regexp.MustCompile(`(?i)\Aweather`)

	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			client.SendText(evt.RoomID, "Please wait while I fetch the forecast for you...")
			forecast, err := WeatherForecast(ctx, cityCode, FormatFeed)
			if err != nil {
				client.SendText(evt.RoomID, fmt.Sprintf("I'm unable to retrieve the forecast. 😔 (%s)", err.Error()))
				return nil
			}

			client.SendText(evt.RoomID, forecast)
			return nil
		},

		predicates.All(
			predicates.MessageMatching(r),
			predicates.NotFromUser(b.UserID),
		),
	)
}

func WeatherForecast(ctx context.Context, cityCode string, formatWeather func(Feed) (string, error)) (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://weather.gc.ca/rss/city/%s_e.xml", cityCode))
	if err != nil {
		return "", fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	var feed Feed
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response: %w", err)
	}
	// TODO error handle
	if err := xml.Unmarshal(data, &feed); err != nil {
		return "", fmt.Errorf("unable to parse XML: %w", err)
	}

	return formatWeather(feed)
}

func FormatFeed(feed Feed) (string, error) {
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

	return builder.String(), nil
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
