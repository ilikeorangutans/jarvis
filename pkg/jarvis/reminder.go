package jarvis

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	sq "github.com/Masterminds/squirrel"
)

var (
	cancelRegex        = regexp.MustCompile(`(?i)\A\s*cancel\s+reminder\s+([0-9]+)`)
	listRegex          = regexp.MustCompile(`(?i)\A\s*reminders`)
	messageRegex       = regexp.MustCompile(`(?i)\A\s*remind\s+me\s+(.*)`)
	timeSpecifierRegex = regexp.MustCompile(`(?i)(this|next|on|every)?\s*(today|tomorrow|day|monday|tuesday|wednesday|thursday|friday|saturday|sunday|weekday)?(\s*(at\s+([0-9]{1,2}):?([0-9]{2})?(am|pm)?|morning|noon|afternoon|evening|night))?(\s+.*)?`)
)

func NewReminders(ctx context.Context, b *bot.Bot, c *cron.Cron, db sqlx.Ext) (*Reminders, error) {
	return &Reminders{
		c:  c,
		b:  b,
		db: db,
	}, nil
}

type Reminders struct {
	c  *cron.Cron
	b  *bot.Bot
	db sqlx.Ext
}

func (r *Reminders) Start(ctx context.Context) error {
	var reminders []*Reminder
	if err := sqlx.Select(r.db, &reminders, "select * from reminders"); err != nil {
		return err
	}
	log.Info().Int("count", len(reminders)).Msg("rescheduling reminders")
	for _, reminder := range reminders {
		r.schedule(ctx, reminder)
	}

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

	return nil
}

func (r *Reminders) Add(ctx context.Context, reminder *Reminder) error {
	log.Info().Msgf("adding reminder %s, %s", reminder.EffectiveDay(), reminder.Day)
	result, err := sq.
		Insert("reminders").
		Columns("recurring", "minute", "hour", "day", "message", "room", "user", "created_at").
		Values(reminder.Recurring, reminder.Minute, reminder.Hour, reminder.EffectiveDay(), reminder.Message, reminder.Room, reminder.User, reminder.CreatedAt).
		RunWith(r.db).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("could not insert record: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("could not get last inserted id: %w", err)
	}
	reminder.ID = id

	r.schedule(ctx, reminder)

	return nil
}

func (r *Reminders) FindByID(ctx context.Context, id int64) (*Reminder, error) {
	reminder := &Reminder{}
	sql, args := sq.Select("*").From("reminders").Where(sq.Eq{"id": id}).Limit(1).MustSql()
	err := sqlx.Get(r.db, reminder, sql, args...)

	return reminder, err
}

func (r *Reminders) Update(ctx context.Context, reminder *Reminder) error {
	_, err := sq.
		Update("reminders").
		Set("entry_id", reminder.EntryID).
		Where(sq.Eq{"id": reminder.ID}).
		RunWith(r.db).
		ExecContext(ctx)

	return err
}
func (r *Reminders) schedule(ctx context.Context, reminder *Reminder) {
	spec := reminder.ToSpec()
	log.Info().Str("spec", spec).Send()
	entryID, err := r.c.AddFunc(spec, func() {
		r.sendReminder(reminder)
	})
	if err != nil {
		log.Error().Err(err).Msg("could not schedule")
	}

	reminder.EntryID = &entryID
	if err := r.Update(ctx, reminder); err != nil {
		log.Error().Err(err).Msg("could not update")
	}
}

func (r *Reminders) sendReminder(reminder *Reminder) {
	log.Info().Str("user-id", reminder.User.String()).Str("reminder", reminder.Message).Msg("sending reminder")
	user, _, _ := reminder.User.Parse()
	r.b.Client().SendText(reminder.Room, fmt.Sprintf("🗓️ %s, reminding you %s", user, reminder.Message))

	if !reminder.Recurring {
		go r.Remove(context.Background(), reminder.ID)
	}
}

func (r *Reminders) Remove(ctx context.Context, id int64) error {
	log.Debug().Int64("id", id).Msg("removing reminder")

	reminder, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if reminder.EntryID != nil {
		log.Info().Int64("id", reminder.ID).Msg("unscheduling")
		r.c.Remove(*reminder.EntryID)
	}

	res, err := sq.Delete("reminders").Where(sq.Eq{"id": id}).RunWith(r.db).Exec()
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no rows were removed")
	}
	return nil
}

func (r *Reminders) List(userID id.UserID) ([]*Reminder, error) {
	var reminders []*Reminder
	sql, args, err := sq.Select("*").
		From("reminders").
		Where(sq.Eq{"user": userID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("could not build query: %w", err)
	}

	if err := sqlx.Select(r.db, &reminders, sql, args...); err != nil {
		return nil, err
	}

	return reminders, nil
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
				reminder.User = evt.Sender
				reminder.Room = evt.RoomID
				reminder.CreatedAt = time.Now()

				if err := reminders.Add(ctx, reminder); err != nil {
					return err
				}

				client.SendText(evt.RoomID, fmt.Sprintf("🗓️ New reminder (%d) %s", reminder.ID, reminder))
			}

			return nil
		},
		predicates.All(
			predicates.MessageMatching(messageRegex),
		),
	)
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			msg := evt.Content.AsMessage()
			parts := cancelRegex.FindStringSubmatch(msg.Body)
			id, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return err
			}
			if err := reminders.Remove(ctx, id); err != nil {
				client.SendText(evt.RoomID, fmt.Sprintf("Terribly sorry, but I couldn't cancel your reminder %d: %s", id, err))
				return err
			} else {
				client.SendText(evt.RoomID, fmt.Sprintf("✅ Very good, I've cancelled your reminder %d.", id))
			}

			return nil
		},
		predicates.All(
			predicates.MessageMatching(cancelRegex),
		),
	)
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			counter := 0
			sb := strings.Builder{}
			list, err := reminders.List(evt.Sender)
			if err != nil {
				return err
			}
			user, _, _ := evt.Sender.Parse()
			if len(list) == 0 {
				client.SendText(evt.RoomID, fmt.Sprintf("🗓️ I have no reminders for you, %s", user))
				return nil
			}

			client.SendText(evt.RoomID, fmt.Sprintf("🗓️ I have %d reminders for you, %s", len(list), user))
			for _, reminder := range list {
				counter++
				sb.WriteString(strconv.FormatInt(reminder.ID, 10))
				sb.WriteString(". ")
				if reminder.Recurring {
					sb.WriteString("every")
					sb.WriteString(" ")
				}

				sb.WriteString(reminder.Day)
				sb.WriteString(" at ")
				sb.WriteString(reminder.Hour)
				if reminder.Minute != "" {
					sb.WriteString(":")
					sb.WriteString(reminder.Minute)
				}
				sb.WriteString(": ")
				sb.WriteString(reminder.Message)

				sb.WriteString("\n")
				if counter > 5 {
					client.SendText(evt.RoomID, sb.String())
					sb.Reset()
				}
			}

			if sb.Len() > 0 {
				client.SendText(evt.RoomID, sb.String())
			}
			client.SendHTML(evt.RoomID, "To cancel a reminder, message me like so: <tt>cancel reminder 12</tt>")
			return nil
		},
		predicates.All(
			predicates.MessageMatching(listRegex),
		),
	)

	return nil
}

func ReminderFromParts(parsed []string) (*Reminder, error) {
	recurring := parsed[1] == "every"
	day := strings.TrimSpace(parsed[2])
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
	return &Reminder{
		Recurring: recurring,
		Minute:    minute,
		Hour:      hour,
		Day:       day,
		Message:   strings.TrimSpace(parsed[8]),
	}, nil
}

type Reminder struct {
	ID        int64
	CreatedAt time.Time `db:"created_at"`
	Recurring bool
	Minute    string
	Hour      string
	Day       string
	Message   string
	Room      id.RoomID
	User      id.UserID
	EntryID   *cron.EntryID `db:"entry_id"`
}

// ResolveDay takes relative day specifiers, like "tomorrow" and resolves them to a specific day of the week.
func (r *Reminder) ResolveRelativeDay(t time.Time) string {
	switch r.Day {
	case "":
		hour, err := strconv.Atoi(r.Hour)
		if err != nil {
			panic(err)
		}
		minute, err := strconv.Atoi(r.Minute)
		if err != nil {
			panic(err)
		}
		if t.Hour() > hour && t.Minute() > minute {
			return tomorrow(t)
		} else {
			return today(t)
		}
	case "today":
		return today(t)
	case "tomorrow":
		return tomorrow(t)
	default:
		return r.Day
	}
}

func today(t time.Time) string {
	return strings.ToLower(t.Weekday().String())
}

func tomorrow(t time.Time) string {
	return strings.ToLower(t.AddDate(0, 0, 1).Weekday().String())
}

func (r *Reminder) EffectiveDay() string {
	location, _ := time.LoadLocation("EST")
	now := time.Now().In(location)
	resolved := r.ResolveRelativeDay(now)
	switch resolved {
	case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		return resolved
	default:
		return resolved
	}
}

func (r *Reminder) ToSpecDay() string {
	effective := r.EffectiveDay()
	switch effective {
	case "weekday":
		return "MON,TUE,WED,THU,FRI"
	case "day":
		return "*"
	default:
		return strings.ToUpper(effective[0:3])
	}
}

func (r *Reminder) ToSpec() string {
	spec := []string{}
	spec = append(spec, r.Minute)      // minute
	spec = append(spec, r.Hour)        // hour
	spec = append(spec, "*")           // day of month
	spec = append(spec, "*")           // month
	spec = append(spec, r.ToSpecDay()) // weekday

	return strings.Join(spec, " ")
}

func (r *Reminder) String() string {
	parts := []string{}
	if r.Recurring {
		parts = append(parts, "every")
	}
	parts = append(parts, r.EffectiveDay())
	parts = append(parts, "at")
	parts = append(parts, fmt.Sprintf("%s:%s:", r.Hour, r.Minute))
	parts = append(parts, r.Message)

	return strings.Join(parts, " ")
}
