package jarvis

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/jasonlvhit/gocron"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	listRegex          = regexp.MustCompile(`(?i)\A\s*reminders`)
	messageRegex       = regexp.MustCompile(`(?i)\A\s*remind\s+me\s+(.*)`)
	timeSpecifierRegex = regexp.MustCompile(`(?i)(this|next|on|every)?\s*(tomorrow|day|monday|tuesday|wednesday|thursday|friday|saturday|sunday|weekday)(\s+(at\s+([0-9]{1,2}):?([0-9]{2})?(am|pm)?|morning|noon|afternoon|evening|night))?(\s+to\s+.*)?`)
)

func NewReminders(b *bot.Bot, c *cron.Cron) (*Reminders, error) {
	return &Reminders{
		c:    c,
		b:    b,
		list: make(map[id.UserID][]*reminder),
	}, nil
}

type Reminders struct {
	c    *cron.Cron
	b    *bot.Bot
	list map[id.UserID][]*reminder
}

func (r *Reminders) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("shutting down cron")
				r.c.Stop()
				return
			}
		}
	}()
}

func (r *Reminders) Add(reminder *reminder) {
	r.list[reminder.user] = append(r.list[reminder.user], reminder)
	if reminder.recurring {
		r.schedule(reminder)
	} else {

	}
}

func (r *Reminders) schedule(reminder *reminder) {
	// crontab format: minutes / hours / day of month / month / day of week

	spec := []string{}
	spec = append(spec, reminder.minute)
	spec = append(spec, reminder.hour)
	spec = append(spec, "*")
	spec = append(spec, "*")

	switch reminder.day {
	case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		spec = append(spec, strings.ToUpper(reminder.day[0:3]))
	case "weekday":
		spec = append(spec, "MON,TUE,WED,THU,FRI")
	default:
		spec = append(spec, "*")
	}

	log.Info().Strs("spec", spec).Send()
	_, err := r.c.AddFunc(strings.Join(spec, " "), func() {
		r.sendReminder(reminder)
	})

	if err != nil {
		log.Error().Err(err).Msg("could not schedule")
	}
}

func (r *Reminders) sendReminder(reminder *reminder) {
	log.Info().Str("user-id", reminder.user.String()).Str("reminder", reminder.message).Msg("sending reminder")
	user, _, _ := reminder.user.Parse()
	r.b.Client().SendText(reminder.room, fmt.Sprintf("🗓️ %s, reminding you %s", user, reminder.message))
}

func (r *Reminders) List(userID id.UserID) []*reminder {
	return r.list[userID]
}

func AddReminderHandlers(ctx context.Context, b *bot.Bot, reminders *Reminders) error {

	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			msg := evt.Content.AsMessage()
			parts := messageRegex.FindStringSubmatch(msg.Body)
			user, _, _ := evt.Sender.Parse()
			if len(parts) == 0 {
				client.SendHTML(evt.RoomID, fmt.Sprintf(`%s I can remind you of stuff`, user))
				return nil
			}
			command := parts[1]
			if timeSpecifierRegex.MatchString(command) {
				parsed := timeSpecifierRegex.FindStringSubmatch(command)

				reminder, err := ReminderFromParts(parsed)
				if err != nil {
					return err
				}
				log.Info().Msgf("%v", reminder)
				reminder.user = evt.Sender
				reminder.room = evt.RoomID

				reminders.Add(reminder)
			}

			client.SendReaction(evt.RoomID, evt.ID, "🗓️")
			return nil
		},
		predicates.All(
			predicates.MessageMatching(messageRegex),
		),
	)
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			counter := 0
			sb := strings.Builder{}
			list := reminders.List(evt.Sender)
			user, _, _ := evt.Sender.Parse()
			if len(list) == 0 {
				client.SendText(evt.RoomID, fmt.Sprintf("🗓️ I have no reminders for you, %s", user))
				return nil
			}

			client.SendText(evt.RoomID, fmt.Sprintf("🗓️ I have %d reminders for you, %s", len(list), user))
			for _, reminder := range list {
				counter++
				sb.WriteString("- ")
				sb.WriteString(reminder.room.String())

				if counter > 5 {
					client.SendText(evt.RoomID, sb.String())
					sb.Reset()
				}
			}

			if sb.Len() > 0 {
				client.SendText(evt.RoomID, sb.String())
			}
			return nil
		},
		predicates.All(
			predicates.MessageMatching(listRegex),
		),
	)

	return nil
}

func ReminderFromParts(parsed []string) (*reminder, error) {
	recurring := parsed[1] == "every"
	day := parsed[2]
	hour := "08"
	minute := "00"
	fuzzyTime := !strings.HasPrefix(parsed[4], "at")
	if fuzzyTime {
		switch parsed[4] {
		case "morning":
			hour = "08"
		case "noon":
			hour = "12"
		case "afternoon":
			hour = "15"
		case "evening":
			hour = "18"
		case "night":
			hour = "21"
		}
	} else if parsed[4] != "" {
		twelveHour := false
		if parsed[7] == "am" || parsed[7] == "pm" {
			twelveHour = true
		}

		if twelveHour {
			i, _ := strconv.Atoi(parsed[5])
			hour = strconv.Itoa(i + 12)
		} else {
			hour = parsed[5]
		}

		hasMinutes := parsed[6] != ""
		if hasMinutes {
			minute = parsed[6]
		}
	}
	return &reminder{
		recurring: recurring,
		minute:    minute,
		hour:      hour,
		day:       day,
		message:   strings.TrimSpace(parsed[8]),
	}, nil
}

type reminder struct {
	recurring bool
	minute    string
	hour      string
	day       string
	message   string
	room      id.RoomID
	user      id.UserID
}

func (r *reminder) Apply(scheduler *gocron.Scheduler) {
}

func reminderTask(ctx context.Context, b *bot.Bot) func() {
	log.Info().Msg("starting reminder task")
	return func() {
	}

}
