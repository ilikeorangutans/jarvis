package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/ilikeorangutans/remind-me-bot/pkg/bot"
	"github.com/ilikeorangutans/remind-me-bot/pkg/jarvis"
	"github.com/ilikeorangutans/remind-me-bot/pkg/predicates"
	"github.com/ilikeorangutans/remind-me-bot/pkg/version"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	FancyLogs     bool `split_words:"true"`
	Debug         bool
	HomeserverURL *url.URL `split_words:"true" required:"true"`
	UserID        string   `split_words:"true" required:"true"`
	Password      string   `split_words:"true" required:"true"`
	DataPath      string   `split_words:"true" required:"true"`
}

func main() {
	var config Config
	if err := envconfig.Process("jarvis", &config); err != nil {
		log.Fatal().Err(err).Send()
	}

	if config.FancyLogs {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if config.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	startTime := time.Now()
	log.Info().Str("log-level", zerolog.GlobalLevel().String()).Str("sha", version.SHA).Str("build-time", version.BuildTime).Str("data-path", config.DataPath).Str("homeserverURL", config.HomeserverURL.String()).Str("userID", config.UserID).Msg("Jarvis starting up")

	db, err := bolt.Open(filepath.Join(config.DataPath, "reminder-bot.db"), 0666, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("opening database failed")
	}
	defer db.Close()

	botConfig := bot.BotConfiguration{
		Password:      config.Password,
		HomeserverURL: config.HomeserverURL,
		Username:      config.UserID,
	}

	botStorage, err := bot.NewBoltBotStorage("jarvis", db)
	b, err := bot.NewBot(botConfig, botStorage)
	ctx, cancel := context.WithCancel(context.Background())

	if err := b.Authenticate(ctx); err != nil {
		log.Fatal().Err(err).Msg("authentication failed")
	}

	jarvis.AddDiceHandler(b)
	jarvis.AddWeatherHandler(ctx, b)
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			t, err := time.Parse("2006-01-02T15:04:05-0700", version.BuildTime)
			if err != nil {
				log.Error().Err(err).Send()
			}

			client.SendNotice(
				evt.RoomID,
				fmt.Sprintf("running since %s, sha %s, build time %s (%s)", humanize.Time(startTime), version.SHA, humanize.Time(t), version.BuildTime),
			)
			return nil
		},
		predicates.MessageMatching(regexp.MustCompile("status")),
		predicates.AtUser(id.UserID(config.UserID)),
	)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		for {
			select {
			case <-signals:
				log.Info().Msg("received interrupt signal")
				cancel()
			}
		}
	}()

	err = b.Run(ctx)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
}

//func foo() {
//	deviceID := ""
//	err = db.View(func(tx *bolt.Tx) error {
//		bucket := tx.Bucket([]byte("bot"))
//		if bucket != nil {
//			deviceID = string(bucket.Get([]byte("device-id")))
//		}
//		return nil
//	})
//	if err != nil {
//		log.Fatal().Err(err).Msgf("loading device id")
//	}
//
//	client, err := mautrix.NewClient(homeserverURL, "", "")
//	if err != nil {
//		log.Fatal().Err(err).Msgf("connecting to homeserver %s failed", homeserverURL)
//	}
//
//	loginResp, err := client.Login(&mautrix.ReqLogin{
//		Type:             mautrix.AuthTypePassword,
//		Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: userID},
//		Password:         password,
//		StoreCredentials: true,
//		DeviceID:         id.DeviceID(deviceID),
//	})
//	if err != nil {
//		log.Fatal().Err(err).Msg("authentication failed")
//	}
//
//	if loginResp.DeviceID.String() != deviceID {
//		deviceID = loginResp.DeviceID.String()
//		log.Info().Str("deviceID", deviceID).Msg("new device id")
//
//		err = db.Update(func(tx *bolt.Tx) error {
//			bucket, err := tx.CreateBucketIfNotExists([]byte("bot"))
//			if err != nil {
//				return err
//			}
//			return bucket.Put([]byte("device-id"), []byte(deviceID))
//		})
//		if err != nil {
//			log.Fatal().Err(err).Msgf("saving device id")
//		}
//	}
//
//	log.Info().Str("device-id", loginResp.DeviceID.String()).Str("user-id", loginResp.UserID.String()).Msg("login successful")
//
//	queue := make(chan bot.Reminder, 10)
//	go func() {
//		// TODO needs a context here
//		for {
//			select {
//			case reminder := <-queue:
//				log.Info().Str("user-id", reminder.User.String()).Str("reminder", reminder.Message).Msg("got reminder")
//				duration := time.Until(reminder.When)
//				time.AfterFunc(duration, func() {
//					user, _, _ := reminder.User.Parse()
//					client.SendText(reminder.Room, fmt.Sprintf("%s, reminding you from %s of %s", user, humanize.Time(reminder.Timestamp), reminder.Message))
//				})
//			}
//		}
//	}()
//
//	store, err := bot.NewClientStore(db)
//	if err != nil {
//		log.Fatal().Err(err).Msg("creating bot store")
//	}
//	client.Store = store
//	client.SyncPresence = event.PresenceOnline
//
//	joinedRooms := make(map[id.RoomID]struct{})
//	if rooms, err := client.JoinedRooms(); err != nil {
//		log.Fatal().Err(err).Msg("failed listing joined rooms")
//	} else {
//		for _, roomID := range rooms.JoinedRooms {
//			log.Info().Str("room-id", roomID.String()).Msg("joined room")
//			joinedRooms[roomID] = struct{}{}
//		}
//	}
//
//	syncer := client.Syncer.(*mautrix.DefaultSyncer)
//	reminderRegex := regexp.MustCompile("\\A\\s*in\\s+([0-9]+)\\s+(second|minute|hour|day|week|month|year)s?\\s+(.*)\\z")
//	syncer.OnEventType(event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
//		client.MarkRead(evt.RoomID, evt.ID)
//		message := evt.Content.AsMessage()
//
//		if evt.Timestamp < startTime {
//			return
//		}
//
//		if strings.HasPrefix(strings.ToLower(message.Body), userID) {
//			sub := message.Body[len(userID):len(message.Body)]
//			switch strings.TrimSpace(sub) {
//			case "version":
//				t, err := time.Parse("2006-01-02T15:04:05-0700", version.BuildTime)
//				if err != nil {
//					log.Error().Err(err).Send()
//				}
//
//				client.SendNotice(evt.RoomID, fmt.Sprintf("sha %s, build time %s (%s)", version.SHA, humanize.Time(t), version.BuildTime))
//			case "help":
//				// TODO trying to get html messages to work
//				client.SendMessageEvent(evt.RoomID, event.EventMessage, event.MessageEventContent{
//					MsgType:       event.MsgText,
//					Body:          "I can remind you of things in the future. Just write a message like this: `remind me in 1 hour how cool this is`",
//					FormattedBody: "I can remind you of things in the future. Just write a message like this: <tt>remind me in 1 hour how cool this is</tt>",
//					Format:        event.FormatHTML,
//				})
//			case "love you":
//				user, _, _ := evt.Sender.Parse()
//				client.SendText(evt.RoomID, fmt.Sprintf("I love you too, %s! ❤️", user))
//			case "spam":
//				for i := 0; i < 20; i++ {
//					client.SendText(evt.RoomID, "spam")
//				}
//			default:
//				user, _, _ := evt.Sender.Parse()
//				client.SendText(evt.RoomID, fmt.Sprintf("Hi %s!", user))
//			}
//			return
//		}
//		// TODO idea: catch everything with prefix  remind and the parse user. me is current user, or other user
//		if !strings.HasPrefix(strings.ToLower(message.Body), "remind me") {
//			return
//		}
//
//		input := message.Body[9:len(message.Body)]
//		match := reminderRegex.FindStringSubmatch(input)
//
//		if len(match) == 0 {
//			client.SendText(evt.RoomID, fmt.Sprint("Sorry, I did not understand your request. You can ask me to remind you like this: remind me in 1 hour how awesome you are"))
//			return
//		}
//		num, _ := strconv.Atoi(match[1])
//		amount := time.Duration(num)
//		var unit time.Duration
//		switch match[2] {
//		case "minute":
//			unit = time.Minute
//		case "hour":
//			unit = time.Hour
//		case "day":
//			unit = time.Hour * 24
//		case "week":
//			unit = time.Hour * 24 * 7
//		case "month":
//			unit = time.Hour * 24 * 30
//		case "year":
//			unit = time.Hour * 24 * 365
//		default:
//			unit = time.Second
//		}
//		duration := amount * unit
//		msg := match[3]
//		when := time.Now().Add(duration).Add(1 * time.Second) // Add one second so the humanized time, which is rounded, is closer to the expected value.
//
//		resp, err := client.SendText(evt.RoomID, fmt.Sprintf("I'll remind you in %s: %s", humanize.Time(when), msg))
//		if err != nil {
//			log.Error().Err(err).Msg("sending message")
//		}
//
//		reminder := bot.Reminder{
//			EventID:   resp.EventID,
//			Message:   msg,
//			When:      when,
//			User:      evt.Sender,
//			Room:      evt.RoomID,
//			Timestamp: time.Now(),
//		}
//
//		queue <- reminder
//	})
//	syncer.OnEventType(event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
//		membership := evt.Content.AsMember()
//		switch membership.Membership {
//		case event.MembershipInvite:
//			if _, ok := joinedRooms[evt.RoomID]; ok {
//				return
//			}
//			log.Info().Str("room-id", evt.RoomID.String()).Bool("invite", true).Str("sender", evt.Sender.String()).Msgf("invite to room")
//			if _, err := client.JoinRoomByID(evt.RoomID); err != nil {
//				log.Error().Err(err)
//				return
//			}
//			joinedRooms[evt.RoomID] = struct{}{}
//			if _, err := client.SendNotice(evt.RoomID, fmt.Sprintf("%s has joined this room", userID)); err != nil {
//				log.Error().Err(err)
//				return
//			}
//
//		case event.MembershipLeave:
//			delete(joinedRooms, evt.RoomID)
//			log.Info().Str("room-id", evt.RoomID.String()).Msg("leaving room")
//		default:
//			// ignore
//		}
//	})
//
//	shutdown := make(chan os.Signal, 1)
//	signal.Notify(shutdown, os.Interrupt)
//
//	go func() {
//		http.HandleFunc("/services/ping", func(w http.ResponseWriter, r *http.Request) {
//			w.Write([]byte("pong"))
//		})
//		http.ListenAndServe(":8080", nil)
//	}()
//
//	go func() {
//		for {
//			log.Info().Msg("starting sync")
//			err := client.Sync()
//			if errors.Is(err, mautrix.MLimitExceeded) {
//				log.Warn().Err(err).Msg("limit exceeded, backing off")
//				time.Sleep(10 * time.Second)
//			} else if err != nil {
//				log.Fatal().Err(err).Msg("sync failed")
//			}
//		}
//	}()
//
//	<-shutdown
//	log.Info().Msg("shutting down")
//}
